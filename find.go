package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const browsePageSize = 12

type FindOptions struct {
	Query      string
	JSON       bool
	Raw        bool
	Synthesize bool
	Provider   AIProvider
}

type scoredCanonicalNote struct {
	note          *CanonicalNote
	score         int
	overlap       float64
	sharedTerms   int
	exactID       bool
	exactTitle    bool
	exactAlias    bool
	exactPath     bool
	titleContains bool
	aliasContains bool
	summaryHit    bool
	bodyHit       bool
	topicHit      bool
}

type FindResult struct {
	Query        string                `json:"query"`
	Mode         string                `json:"mode"`
	Provider     string                `json:"provider,omitempty"`
	Model        string                `json:"model,omitempty"`
	Summary      string                `json:"summary,omitempty"`
	Selected     *FindNoteOutput       `json:"selected,omitempty"`
	BrowseTopics []FindTopicOutput     `json:"browse_topics,omitempty"`
	Candidates   []FindCandidateOutput `json:"candidates,omitempty"`
}

type FindNoteOutput struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Aliases        []string `json:"aliases,omitempty"`
	Summary        string   `json:"summary,omitempty"`
	Topics         []string `json:"topics,omitempty"`
	Materialized   string   `json:"materialized_path"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
	SourceCaptures []string `json:"source_captures,omitempty"`
}

type FindCandidateOutput struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Aliases  []string `json:"aliases,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Topics   []string `json:"topics,omitempty"`
	Path     string   `json:"path"`
	Score    int      `json:"score"`
	Overlap  float64  `json:"overlap"`
	Match    string   `json:"match"`
	Selected bool     `json:"selected,omitempty"`
}

type FindTopicOutput struct {
	Topic string `json:"topic"`
	Count int    `json:"count"`
}

func parseFindOptions(args []string, config *Config) (*FindOptions, error) {
	provider, err := ParseAIProvider(config.AIProvider)
	if err != nil {
		return nil, err
	}

	options := &FindOptions{
		Provider: provider,
	}

	var queryParts []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			options.JSON = true
		case "--raw":
			options.Raw = true
		case "--synthesize", "--summary":
			options.Synthesize = true
		case "--provider":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for --provider")
			}
			provider, err := ParseAIProvider(args[i+1])
			if err != nil {
				return nil, err
			}
			options.Provider = provider
			i++
		default:
			if strings.HasPrefix(arg, "--") {
				return nil, fmt.Errorf("unknown flag: %s", arg)
			}
			queryParts = append(queryParts, arg)
		}
	}

	if options.Raw && options.Synthesize {
		return nil, fmt.Errorf("--raw cannot be combined with --synthesize")
	}

	options.Query = strings.TrimSpace(strings.Join(queryParts, " "))
	if options.Query == "" && options.Synthesize {
		return nil, fmt.Errorf("--synthesize requires a query")
	}

	return options, nil
}

func resolveCanonicalFind(kbPath string, options *FindOptions) (*CanonicalNote, []scoredCanonicalNote, bool, error) {
	notes, err := loadCanonicalNotes(kbPath)
	if err != nil {
		return nil, nil, false, err
	}
	if len(notes) == 0 {
		return nil, nil, false, nil
	}

	ranked := rankCanonicalNotesForFind(notes, options.Query)
	if len(ranked) == 0 {
		return nil, nil, false, nil
	}

	selected, confident := selectBestCanonicalFindCandidate(ranked)
	if selected != nil && (confident || options.Synthesize) {
		if !options.Synthesize || confident {
			return selected, ranked, confident, nil
		}
	}

	if options.Synthesize {
		return nil, ranked, false, nil
	}

	if selected != nil && confident {
		return selected, ranked, confident, nil
	}

	if isTerminal(os.Stdin) && !options.JSON {
		selected = promptForCanonicalFindCandidate(options.Query, ranked)
		if selected != nil {
			return selected, ranked, true, nil
		}
		return nil, ranked, false, nil
	}

	return nil, ranked, false, nil
}

func resolveBrowseFind(kbPath string, options *FindOptions) (*CanonicalNote, []scoredCanonicalNote, []FindTopicOutput, error) {
	manifest, err := loadCanonicalNoteManifest(kbPath)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(manifest) == 0 {
		return nil, nil, nil, nil
	}

	notes := canonicalBrowseNotesFromManifest(manifest)
	candidates, topics := buildBrowseCandidates(notes)
	if len(candidates) == 0 {
		return nil, nil, topics, nil
	}

	return nil, candidates, topics, nil
}

func canonicalBrowseNotesFromManifest(entries []*CanonicalNoteManifestEntry) []*CanonicalNote {
	notes := make([]*CanonicalNote, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		notes = append(notes, &CanonicalNote{
			ID:               entry.ID,
			Title:            entry.Title,
			Aliases:          compactStrings(entry.Aliases),
			Summary:          strings.TrimSpace(entry.Summary),
			Topics:           compactStrings(entry.Topics),
			SourceCaptureIDs: compactStrings(entry.SourceCaptureIDs),
			MaterializedPath: entry.MaterializedPath,
			CreatedAt:        entry.CreatedAt,
			UpdatedAt:        entry.UpdatedAt,
			Revision:         entry.Revision,
		})
	}
	return notes
}

func buildBrowseCandidates(notes []*CanonicalNote) ([]scoredCanonicalNote, []FindTopicOutput) {
	candidates := make([]scoredCanonicalNote, 0, len(notes))
	topicCounts := map[string]int{}

	for _, note := range notes {
		if note == nil {
			continue
		}

		candidates = append(candidates, scoredCanonicalNote{
			note:  note,
			score: 1,
		})

		seenTopics := map[string]struct{}{}
		topics := compactStrings(note.Topics)
		if len(topics) == 0 {
			topics = []string{deriveNoteCategory(note)}
		}

		for _, topic := range topics {
			topic = strings.TrimSpace(topic)
			if topic == "" {
				continue
			}

			key := strings.ToLower(topic)
			if _, exists := seenTopics[key]; exists {
				continue
			}
			seenTopics[key] = struct{}{}
			topicCounts[topic]++
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		left := browseNoteTime(candidates[i].note)
		right := browseNoteTime(candidates[j].note)
		if left.Equal(right) {
			return candidates[i].note.Title < candidates[j].note.Title
		}
		return left.After(right)
	})

	for i := range candidates {
		candidates[i].score = len(candidates) - i
	}

	topics := make([]FindTopicOutput, 0, len(topicCounts))
	for topic, count := range topicCounts {
		topics = append(topics, FindTopicOutput{
			Topic: topic,
			Count: count,
		})
	}

	sort.Slice(topics, func(i, j int) bool {
		if topics[i].Count == topics[j].Count {
			return topics[i].Topic < topics[j].Topic
		}
		return topics[i].Count > topics[j].Count
	})

	return candidates, topics
}

func browseNoteTime(note *CanonicalNote) time.Time {
	if note == nil {
		return time.Time{}
	}
	if !note.UpdatedAt.IsZero() {
		return note.UpdatedAt
	}
	return note.CreatedAt
}

func rankCanonicalNotesForFind(notes []*CanonicalNote, query string) []scoredCanonicalNote {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	lowerQuery := strings.ToLower(query)
	querySlug := slugifyTitle(query)
	ranked := make([]scoredCanonicalNote, 0, len(notes))

	for _, note := range notes {
		if note == nil {
			continue
		}

		candidate := scoredCanonicalNote{
			note: note,
		}

		if strings.EqualFold(note.ID, query) {
			candidate.exactID = true
			candidate.score += 1000
		}
		if strings.EqualFold(note.Title, query) {
			candidate.exactTitle = true
			candidate.score += 900
		}
		titleSlug := slugifyTitle(note.Title)
		if querySlug != "" && titleSlug == querySlug {
			candidate.exactTitle = true
			candidate.score += 880
		}

		baseName := filepath.Base(note.MaterializedPath)
		baseStem := strings.TrimSuffix(baseName, filepath.Ext(baseName))
		switch {
		case baseName == query || baseName == query+".md":
			candidate.exactPath = true
			candidate.score += 820
		case querySlug != "" && (baseStem == querySlug || strings.HasPrefix(baseStem, querySlug+"-")):
			candidate.exactPath = true
			candidate.score += 820
		case strings.EqualFold(filepath.ToSlash(note.MaterializedPath), filepath.ToSlash(query)):
			candidate.exactPath = true
			candidate.score += 820
		}

		titleLower := strings.ToLower(note.Title)
		if strings.Contains(titleLower, lowerQuery) {
			candidate.titleContains = true
			candidate.score += 220
			if strings.HasPrefix(titleLower, lowerQuery) {
				candidate.score += 40
			}
		}

		for _, alias := range note.Aliases {
			aliasLower := strings.ToLower(alias)
			if strings.EqualFold(alias, query) {
				candidate.exactAlias = true
				candidate.score += 860
			}
			if strings.Contains(aliasLower, lowerQuery) {
				candidate.aliasContains = true
				candidate.score += 180
			}
		}

		if strings.Contains(strings.ToLower(note.Summary), lowerQuery) {
			candidate.summaryHit = true
			candidate.score += 120
		}
		if strings.Contains(strings.ToLower(note.Body), lowerQuery) {
			candidate.bodyHit = true
			candidate.score += 70
		}
		for _, topic := range note.Topics {
			if strings.EqualFold(topic, query) || strings.Contains(strings.ToLower(topic), lowerQuery) {
				candidate.topicHit = true
				candidate.score += 90
			}
		}

		candidate.overlap, candidate.sharedTerms = tokenOverlapStats(note, query)
		candidate.score += candidate.sharedTerms * 14
		if candidate.overlap >= 0.8 {
			candidate.score += 40
		} else if candidate.overlap >= 0.6 {
			candidate.score += 20
		}

		if candidate.score <= 0 {
			continue
		}
		ranked = append(ranked, candidate)
	}

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].note.UpdatedAt.After(ranked[j].note.UpdatedAt)
		}
		return ranked[i].score > ranked[j].score
	})

	return ranked
}

func selectBestCanonicalFindCandidate(candidates []scoredCanonicalNote) (*CanonicalNote, bool) {
	if len(candidates) == 0 {
		return nil, false
	}

	top := candidates[0]
	if top.exactID || top.exactTitle || top.exactAlias || top.exactPath {
		return top.note, true
	}
	if len(candidates) == 1 {
		return top.note, top.score >= 90
	}

	second := candidates[1]
	gap := top.score - second.score

	switch {
	case top.score >= 260 && gap >= 24:
		return top.note, true
	case top.score >= 220 && gap >= 40:
		return top.note, true
	case top.overlap >= 0.8 && gap >= 12:
		return top.note, true
	default:
		return top.note, false
	}
}

func promptForCanonicalFindCandidate(query string, candidates []scoredCanonicalNote) *CanonicalNote {
	if len(candidates) == 0 {
		return nil
	}

	limit := minInt(len(candidates), 8)
	fmt.Printf(Header("Multiple note matches found for '%s':")+"\n", query)
	for i, candidate := range candidates[:limit] {
		line := fmt.Sprintf("  %s %s %s", Dim(fmt.Sprintf("%d.", i+1)), Bold(candidate.note.Title), Gray(candidate.note.ID))
		if summary := strings.TrimSpace(candidate.note.Summary); summary != "" {
			line += " " + Dim("- "+summary)
		}
		fmt.Println(line)
	}

	fmt.Print("\n" + Highlight("Select note number (or 0 to cancel): "))
	var selection int
	fmt.Scanln(&selection)
	if selection < 1 || selection > limit {
		return nil
	}

	return candidates[selection-1].note
}

func candidateMatchLabel(candidate scoredCanonicalNote) string {
	switch {
	case candidate.exactID:
		return "exact-id"
	case candidate.exactTitle:
		return "exact-title"
	case candidate.exactAlias:
		return "exact-alias"
	case candidate.exactPath:
		return "exact-path"
	case candidate.titleContains:
		return "title"
	case candidate.aliasContains:
		return "alias"
	case candidate.topicHit:
		return "topic"
	case candidate.summaryHit:
		return "summary"
	case candidate.bodyHit:
		return "body"
	default:
		return "token-overlap"
	}
}

func buildFindResult(options *FindOptions, selected *CanonicalNote, candidates []scoredCanonicalNote, mode, provider, model, summary string) *FindResult {
	result := &FindResult{
		Query:    options.Query,
		Mode:     mode,
		Provider: provider,
		Model:    model,
		Summary:  strings.TrimSpace(summary),
	}

	if selected != nil {
		result.Selected = &FindNoteOutput{
			ID:             selected.ID,
			Title:          selected.Title,
			Aliases:        compactStrings(selected.Aliases),
			Summary:        strings.TrimSpace(selected.Summary),
			Topics:         compactStrings(selected.Topics),
			Materialized:   selected.MaterializedPath,
			UpdatedAt:      formatOptionalTimestamp(selected.UpdatedAt),
			SourceCaptures: compactStrings(selected.SourceCaptureIDs),
		}
	}

	limit := minInt(len(candidates), 8)
	result.Candidates = make([]FindCandidateOutput, 0, limit)
	for i := 0; i < limit; i++ {
		candidate := candidates[i]
		result.Candidates = append(result.Candidates, FindCandidateOutput{
			ID:       candidate.note.ID,
			Title:    candidate.note.Title,
			Aliases:  compactStrings(candidate.note.Aliases),
			Summary:  strings.TrimSpace(candidate.note.Summary),
			Topics:   compactStrings(candidate.note.Topics),
			Path:     candidate.note.MaterializedPath,
			Score:    candidate.score,
			Overlap:  candidate.overlap,
			Match:    candidateMatchLabel(candidate),
			Selected: selected != nil && selected.ID == candidate.note.ID,
		})
	}

	return result
}

func buildBrowseFindResult(options *FindOptions, selected *CanonicalNote, candidates []scoredCanonicalNote, topics []FindTopicOutput) *FindResult {
	result := buildFindResult(options, selected, candidates, "browse", "", "", "")
	result.BrowseTopics = topics[:minInt(len(topics), 8)]
	return result
}

func printFindJSON(result *FindResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printFindCandidates(query string, candidates []scoredCanonicalNote) {
	if len(candidates) == 0 {
		fmt.Println(Dim("No matching notes found"))
		return
	}

	fmt.Printf(Header("Top matches for '%s':")+"\n", query)
	for i, candidate := range candidates[:minInt(len(candidates), 8)] {
		fmt.Printf("  %s %s %s %s\n",
			Dim(fmt.Sprintf("%d.", i+1)),
			Bold(candidate.note.Title),
			Gray(candidate.note.ID),
			Dim(fmt.Sprintf("[%s score=%d]", candidateMatchLabel(candidate), candidate.score)),
		)
		if summary := strings.TrimSpace(candidate.note.Summary); summary != "" {
			fmt.Printf("     %s\n", Dim(summary))
		}
	}
}

func printBrowseNotes(candidates []scoredCanonicalNote, topics []FindTopicOutput) {
	if len(candidates) == 0 {
		fmt.Println(Dim("No notes available yet"))
		return
	}

	fmt.Printf(Header("Browse notes (%d total):")+"\n", len(candidates))
	if len(topics) > 0 {
		topicParts := make([]string, 0, minInt(len(topics), 8))
		for _, topic := range topics[:minInt(len(topics), 8)] {
			topicParts = append(topicParts, fmt.Sprintf("%s (%d)", topic.Topic, topic.Count))
		}
		fmt.Printf("  %s %s\n", Dim("Topics:"), strings.Join(topicParts, " · "))
	}

	for i := range candidates {
		candidate := candidates[i]
		timestamp := browseNoteTime(candidate.note)
		line := fmt.Sprintf("  %s %s %s", Dim(fmt.Sprintf("%d.", i+1)), Bold(candidate.note.Title), Gray(candidate.note.ID))
		if !timestamp.IsZero() {
			line += " " + Dim(timestamp.Format("2006-01-02"))
		}
		fmt.Println(line)

		if summary := strings.TrimSpace(candidate.note.Summary); summary != "" {
			fmt.Printf("     %s\n", Dim(summary))
		}
	}
}

func browseNotesInteractively(kbPath string, config *Config, options *FindOptions, candidates []scoredCanonicalNote, topics []FindTopicOutput) error {
	if len(candidates) == 0 {
		printBrowseNotes(candidates, topics)
		return nil
	}

	scanner := bufio.NewScanner(os.Stdin)
	currentPage := 0
	pageCount := browsePageCount(len(candidates), browsePageSize)

	for {
		printBrowsePage(candidates, topics, currentPage, browsePageSize)
		fmt.Print("\n" + Highlight("Open note number, n/p page, q quit: "))

		if !scanner.Scan() {
			return scanner.Err()
		}

		action, selectedIndex, nextPage, err := parseBrowseInput(scanner.Text(), currentPage, len(candidates), browsePageSize)
		if err != nil {
			fmt.Println(Dim(err.Error()))
			continue
		}

		switch action {
		case browseActionQuit:
			return nil
		case browseActionNext:
			if nextPage == currentPage && currentPage == pageCount-1 {
				fmt.Println(Dim("Already on the last page"))
				continue
			}
			currentPage = nextPage
		case browseActionPrev:
			if nextPage == currentPage && currentPage == 0 {
				fmt.Println(Dim("Already on the first page"))
				continue
			}
			currentPage = nextPage
		case browseActionSelect:
			note := candidates[selectedIndex].note
			if err := renderResolvedFindNote(kbPath, config, options, note, candidates); err != nil {
				return err
			}
		}
	}
}

func printBrowsePage(candidates []scoredCanonicalNote, topics []FindTopicOutput, page, pageSize int) {
	if len(candidates) == 0 {
		fmt.Println(Dim("No notes available yet"))
		return
	}

	start, end := browsePageBounds(len(candidates), page, pageSize)
	pageCount := browsePageCount(len(candidates), pageSize)

	fmt.Printf(Header("Browse notes (%d total, page %d/%d):")+"\n", len(candidates), page+1, pageCount)
	if len(topics) > 0 {
		topicParts := make([]string, 0, minInt(len(topics), 8))
		for _, topic := range topics[:minInt(len(topics), 8)] {
			topicParts = append(topicParts, fmt.Sprintf("%s (%d)", topic.Topic, topic.Count))
		}
		fmt.Printf("  %s %s\n", Dim("Topics:"), strings.Join(topicParts, " · "))
	}

	fmt.Printf("  %s %d-%d of %d\n", Dim("Showing:"), start+1, end, len(candidates))

	for i := start; i < end; i++ {
		candidate := candidates[i]
		timestamp := browseNoteTime(candidate.note)
		line := fmt.Sprintf("  %s %s %s", Dim(fmt.Sprintf("%d.", i+1)), Bold(candidate.note.Title), Gray(candidate.note.ID))
		if !timestamp.IsZero() {
			line += " " + Dim(timestamp.Format("2006-01-02"))
		}
		fmt.Println(line)

		if summary := strings.TrimSpace(candidate.note.Summary); summary != "" {
			fmt.Printf("     %s\n", Dim(summary))
		}
	}
}

type browseAction int

const (
	browseActionSelect browseAction = iota
	browseActionNext
	browseActionPrev
	browseActionQuit
)

func parseBrowseInput(input string, currentPage, total, pageSize int) (browseAction, int, int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return browseActionQuit, 0, currentPage, fmt.Errorf("enter a note number, n, p, or q")
	}

	switch strings.ToLower(input) {
	case "q", "quit", "exit", "0":
		return browseActionQuit, 0, currentPage, nil
	case "n", "next":
		return browseActionNext, 0, minInt(currentPage+1, maxInt(browsePageCount(total, pageSize)-1, 0)), nil
	case "p", "prev", "previous":
		return browseActionPrev, 0, maxInt(currentPage-1, 0), nil
	}

	selection, err := strconv.Atoi(input)
	if err != nil {
		return browseActionQuit, 0, currentPage, fmt.Errorf("unknown command: %s", input)
	}
	if selection < 1 || selection > total {
		return browseActionQuit, 0, currentPage, fmt.Errorf("note number must be between 1 and %d", total)
	}

	return browseActionSelect, selection - 1, currentPage, nil
}

func browsePageBounds(total, page, pageSize int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if pageSize <= 0 {
		pageSize = browsePageSize
	}

	page = maxInt(0, minInt(page, browsePageCount(total, pageSize)-1))
	start := page * pageSize
	end := minInt(start+pageSize, total)
	return start, end
}

func browsePageCount(total, pageSize int) int {
	if total <= 0 {
		return 0
	}
	if pageSize <= 0 {
		pageSize = browsePageSize
	}
	return (total + pageSize - 1) / pageSize
}

func renderResolvedFindNote(kbPath string, config *Config, options *FindOptions, note *CanonicalNote, candidates []scoredCanonicalNote) error {
	if note == nil {
		return fmt.Errorf("missing note")
	}
	if err := ensureCanonicalNoteMaterialized(kbPath, note); err != nil {
		return err
	}

	if options.JSON {
		return printFindJSON(buildFindResult(options, note, candidates, "note", "", "", ""))
	}

	if options.Raw {
		return printCanonicalNoteRaw(kbPath, note)
	}

	return showEntryWithViewer(config.Viewer, filepath.Join(kbPath, note.MaterializedPath))
}

func printCanonicalNoteRaw(kbPath string, note *CanonicalNote) error {
	if note == nil {
		return fmt.Errorf("missing note")
	}
	if err := ensureCanonicalNoteMaterialized(kbPath, note); err != nil {
		return err
	}
	content, err := os.ReadFile(filepath.Join(kbPath, note.MaterializedPath))
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(content)
	if err == nil && len(content) > 0 && content[len(content)-1] != '\n' {
		_, err = fmt.Fprintln(os.Stdout)
	}
	return err
}

func ensureCanonicalNoteMaterialized(kbPath string, note *CanonicalNote) error {
	if note == nil {
		return fmt.Errorf("missing note")
	}

	entryPath := filepath.Join(kbPath, note.MaterializedPath)
	if note.MaterializedPath != "" {
		if _, err := os.Stat(entryPath); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	return withKBLock(kbPath, func() error {
		entryPath := filepath.Join(kbPath, note.MaterializedPath)
		if note.MaterializedPath != "" {
			if _, err := os.Stat(entryPath); err == nil {
				return nil
			} else if !os.IsNotExist(err) {
				return err
			}
		}

		fullNote, err := loadCanonicalNoteRecord(kbPath, note.ID)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if fullNote != nil {
			*note = *cloneCanonicalNote(fullNote)
		}

		if err := materializeCanonicalNote(kbPath, note); err != nil {
			return err
		}
		return saveCanonicalNoteRecord(kbPath, note)
	})
}

func formatOptionalTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format(time.RFC3339)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
