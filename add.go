package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxCaptureInputBytes    = 512 << 10
	maxPlanningContentChars = 12000
)

type AddOptions struct {
	Provider  AIProvider
	DryRun    bool
	JSON      bool
	FilePath  string
	SourceURL string
	Clipboard bool
}

type AddResult struct {
	CaptureID    string   `json:"capture_id,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	Model        string   `json:"model,omitempty"`
	Summary      string   `json:"summary"`
	Created      []string `json:"created,omitempty"`
	Updated      []string `json:"updated,omitempty"`
	Materialized []string `json:"materialized,omitempty"`
}

func handleAddCommand(args []string) {
	kbPath, err := GetKBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	config, err := LoadConfig(kbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	options, freeArgs, err := parseAddOptions(args, config)
	if err != nil {
		printError("%v", err)
		os.Exit(1)
	}

	content, source, err := collectAddContent(config, kbPath, options, freeArgs)
	if err != nil {
		printError("%v", err)
		os.Exit(1)
	}

	if strings.TrimSpace(content) == "" {
		printWarning("Empty capture, nothing added")
		return
	}

	planningContent := preparePlanningContent(content)

	if options.DryRun {
		notes, err := loadCanonicalNotes(kbPath)
		if err != nil {
			printError("Failed to load notes: %v", err)
			os.Exit(1)
		}

		planningCandidates := selectRelevantNotes(notes, planningContent, 6)
		planned, plannedProvider, plannedModel, planningErr := buildAddPlan(planningContent, planningCandidates, string(options.Provider))
		plan, _, _, err := finalizeAddPlan(planned, planningErr, planningContent, plannedProvider, plannedModel, notes)
		if err != nil {
			printError("Invalid add plan: %v", err)
			os.Exit(1)
		}
		printAddDryRun(plan, options.JSON)
		return
	}

	var result *AddResult
	err = withKBLock(kbPath, func() error {
		currentNotes, err := loadCanonicalNotes(kbPath)
		if err != nil {
			return fmt.Errorf("failed to load notes: %w", err)
		}

		planningCandidates := selectRelevantNotes(currentNotes, planningContent, 6)
		planned, plannedProvider, plannedModel, planningErr := buildAddPlan(planningContent, planningCandidates, string(options.Provider))
		plan, resolvedProvider, model, err := finalizeAddPlan(planned, planningErr, planningContent, plannedProvider, plannedModel, currentNotes)
		if err != nil {
			return err
		}

		state, err := loadOrInitKBState(kbPath)
		if err != nil {
			return fmt.Errorf("failed to initialize KB state: %w", err)
		}

		now := time.Now().UTC()
		reserved := reserveAddIdentifiers(state, plan, now)
		if err := saveKBState(kbPath, state); err != nil {
			return fmt.Errorf("failed to persist reserved KB state: %w", err)
		}

		capture := &CaptureRecord{
			ID:                reserved.CaptureID,
			CreatedAt:         now,
			Source:            source,
			RawContent:        content,
			NormalizedContent: normalizeCaptureContent(content),
			Provider:          resolvedProvider,
			Model:             model,
			OperationID:       reserved.OperationID,
		}

		if err := saveCaptureRecord(kbPath, capture); err != nil {
			return fmt.Errorf("failed to persist capture: %w", err)
		}

		operation := newPendingOperation(reserved.OperationID, capture.ID, resolvedProvider, model, plan, now)
		if err := saveAppliedOperation(kbPath, operation); err != nil {
			return fmt.Errorf("failed to persist pending operation: %w", err)
		}

		appliedResult, err := applyAddPlan(kbPath, config, capture, plan, currentNotes, reserved.NoteIDs, now)
		if err != nil {
			operation.Status = OperationStatusFailed
			operation.Error = err.Error()
			operation.UpdatedAt = time.Now().UTC()
			if saveErr := saveAppliedOperation(kbPath, operation); saveErr != nil {
				return fmt.Errorf("failed to apply capture: %w (also failed to persist operation failure: %v)", err, saveErr)
			}
			return fmt.Errorf("failed to apply capture: %w", err)
		}

		operation.Status = OperationStatusApplied
		operation.Summary = appliedResult.Summary
		operation.AppliedAt = time.Now().UTC()
		operation.UpdatedAt = operation.AppliedAt
		if err := saveAppliedOperation(kbPath, operation); err != nil {
			return fmt.Errorf("failed to persist applied operation: %w", err)
		}

		appliedResult.CaptureID = capture.ID
		appliedResult.Provider = resolvedProvider
		appliedResult.Model = model
		result = appliedResult
		return nil
	})
	if err != nil {
		printError("%v", err)
		os.Exit(1)
	}

	printAddResult(result, options.JSON)
}

func parseAddOptions(args []string, config *Config) (*AddOptions, []string, error) {
	provider, err := ParseAIProvider(config.AIProvider)
	if err != nil {
		return nil, nil, err
	}

	options := &AddOptions{
		Provider: provider,
	}

	var freeArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--provider":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("missing value for --provider")
			}
			provider, err := ParseAIProvider(args[i+1])
			if err != nil {
				return nil, nil, err
			}
			options.Provider = provider
			i++
		case "--file":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("missing value for --file")
			}
			options.FilePath = strings.TrimSpace(args[i+1])
			i++
		case "--url":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("missing value for --url")
			}
			options.SourceURL = strings.TrimSpace(args[i+1])
			i++
		case "--clipboard":
			options.Clipboard = true
		case "--dry-run":
			options.DryRun = true
		case "--json":
			options.JSON = true
		default:
			if strings.HasPrefix(arg, "--") {
				return nil, nil, fmt.Errorf("unknown flag: %s", arg)
			}
			freeArgs = append(freeArgs, arg)
		}
	}

	explicitSources := 0
	if options.FilePath != "" {
		explicitSources++
	}
	if options.SourceURL != "" {
		explicitSources++
	}
	if options.Clipboard {
		explicitSources++
	}
	if explicitSources > 1 {
		return nil, nil, fmt.Errorf("use at most one explicit input source: --file, --url, or --clipboard")
	}
	if explicitSources > 0 && len(freeArgs) > 0 {
		return nil, nil, fmt.Errorf("positional capture text cannot be combined with --file, --url, or --clipboard")
	}

	return options, freeArgs, nil
}

func looksLikeSlug(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func collectAddContent(config *Config, kbPath string, options *AddOptions, args []string) (string, CaptureSource, error) {
	if options != nil {
		switch {
		case options.FilePath != "":
			return readAddContentFromFile(options.FilePath)
		case options.SourceURL != "":
			return readAddContentFromURL(options.SourceURL)
		case options.Clipboard:
			return readAddContentFromClipboard()
		}
	}

	if len(args) > 0 {
		return strings.TrimSpace(strings.Join(args, " ")), CaptureSourceCLI, nil
	}

	if !isTerminal(os.Stdin) {
		data, err := readAllFromStdin()
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(data) != "" {
			return data, CaptureSourceStdin, nil
		}
	}

	return captureWithEditor(config, kbPath)
}

func readAllFromStdin() (string, error) {
	data, err := readLimitedInput(os.Stdin, maxCaptureInputBytes, "stdin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func captureWithEditor(config *Config, kbPath string) (string, CaptureSource, error) {
	tempDir, err := os.MkdirTemp("", "kb-add")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	tempPath := filepath.Join(tempDir, "capture.md")
	if err := os.WriteFile(tempPath, []byte(""), 0644); err != nil {
		return "", "", fmt.Errorf("failed to create temp capture file: %w", err)
	}

	cmd := buildEditorCommand(config, kbPath, tempPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("failed to open editor: %w", err)
	}

	data, err := os.ReadFile(tempPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read edited capture: %w", err)
	}

	return strings.TrimSpace(string(data)), CaptureSourceEditor, nil
}

func buildAddPlan(content string, candidates []*CanonicalNote, provider string) (*AddPlan, string, string, error) {
	if heuristicPlan, ok := planAddHeuristically(content, candidates); ok {
		normalized, err := normalizeAddPlan(heuristicPlan, candidates)
		if err != nil {
			return nil, "", "", err
		}
		return normalized, "heuristic", "deterministic-match", nil
	}

	resolvedProvider, err := ResolveAIProvider(provider)
	if err != nil {
		return nil, "", "", err
	}

	plan, model, err := planAddWithProvider(content, candidates, string(resolvedProvider))
	if err != nil {
		return nil, string(resolvedProvider), "", err
	}

	normalized, err := normalizeAddPlan(plan, candidates)
	if err != nil {
		return nil, string(resolvedProvider), model, err
	}

	return normalized, string(resolvedProvider), model, nil
}

func finalizeAddPlan(plan *AddPlan, planningErr error, content, provider, model string, notes []*CanonicalNote) (*AddPlan, string, string, error) {
	candidates := selectRelevantNotes(notes, content, 6)
	if planningErr != nil {
		printWarning("Organizer unavailable, using local fallback: %v", planningErr)
		localPlan := planAddLocally(content, candidates)
		normalized, err := normalizeAddPlan(localPlan, notes)
		if err != nil {
			return nil, "", "", err
		}
		if err := validateAddPlan(normalized, notes); err != nil {
			return nil, "", "", err
		}
		return normalized, "local", "local-fallback", nil
	}

	normalized, err := normalizeAddPlan(plan, notes)
	if err != nil {
		printWarning("Organizer returned invalid plan, using local fallback: %v", err)
		localPlan := planAddLocally(content, candidates)
		normalized, err = normalizeAddPlan(localPlan, notes)
		if err != nil {
			return nil, "", "", err
		}
		if err := validateAddPlan(normalized, notes); err != nil {
			return nil, "", "", err
		}
		return normalized, "local", "local-fallback", nil
	}
	if err := validateAddPlan(normalized, notes); err != nil {
		printWarning("Organizer returned invalid plan, using local fallback: %v", err)
		localPlan := planAddLocally(content, candidates)
		normalized, err = normalizeAddPlan(localPlan, notes)
		if err != nil {
			return nil, "", "", err
		}
		if err := validateAddPlan(normalized, notes); err != nil {
			return nil, "", "", err
		}
		return normalized, "local", "local-fallback", nil
	}

	return normalized, provider, model, nil
}

func preparePlanningContent(content string) string {
	return clampString(strings.TrimSpace(content), maxPlanningContentChars)
}

func readLimitedInput(reader io.Reader, maxBytes int64, label string) ([]byte, error) {
	limited := io.LimitReader(reader, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", label, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s exceeds the %d KiB capture limit", label, maxBytes/1024)
	}
	return data, nil
}

func planAddLocally(content string, candidates []*CanonicalNote) *AddPlan {
	if heuristicPlan, ok := planAddHeuristically(content, candidates); ok {
		return heuristicPlan
	}

	title := inferNoteTitle(content)
	body := renderCanonicalCaptureMarkdown(content, title)
	if body == "" {
		body = title
	}

	return &AddPlan{
		Summary: "Created a new note from the capture.",
		Actions: []AddAction{
			{
				Type:         "create_note",
				Title:        title,
				Reason:       "Local fallback organizer creates a new canonical note when no AI plan is available.",
				TopicsSet:    inferTopicsFromContent(content),
				BodyMarkdown: body,
				SummarySet:   inferSummaryFromContentWithTitle(content, title),
			},
		},
	}
}

func normalizeAddPlan(plan *AddPlan, notes []*CanonicalNote) (*AddPlan, error) {
	if plan == nil {
		return nil, fmt.Errorf("empty add plan")
	}

	titleToNoteID := make(map[string]string, len(notes))
	for _, note := range notes {
		title := strings.ToLower(strings.TrimSpace(note.Title))
		if title != "" {
			titleToNoteID[title] = note.ID
		}
	}

	normalized := &AddPlan{
		Summary: strings.TrimSpace(plan.Summary),
	}

	updateActions := make(map[string]*AddAction)
	actionOrder := make([]string, 0, len(plan.Actions))

	for _, rawAction := range plan.Actions {
		action := AddAction{
			Type:               strings.ToLower(strings.TrimSpace(rawAction.Type)),
			NoteID:             strings.TrimSpace(rawAction.NoteID),
			Title:              strings.TrimSpace(rawAction.Title),
			Reason:             strings.TrimSpace(rawAction.Reason),
			AliasesAdd:         compactStrings(rawAction.AliasesAdd),
			TopicsSet:          compactStrings(rawAction.TopicsSet),
			SummarySet:         clampString(strings.TrimSpace(rawAction.SummarySet), 500),
			BodyMarkdown:       clampString(strings.TrimSpace(rawAction.BodyMarkdown), 16000),
			BodyAppendMarkdown: clampString(strings.TrimSpace(rawAction.BodyAppendMarkdown), 16000),
		}

		if action.Type == "" {
			continue
		}
		if action.Title == "" && action.Type == "create_note" && action.BodyMarkdown != "" {
			action.Title = inferNoteTitle(action.BodyMarkdown)
		}
		action.Title = clampString(action.Title, 160)

		if action.Type == "create_note" {
			if existingID := titleToNoteID[strings.ToLower(action.Title)]; existingID != "" {
				action.Type = "update_note"
				action.NoteID = existingID
				if action.BodyAppendMarkdown == "" {
					action.BodyAppendMarkdown = action.BodyMarkdown
				}
				action.BodyMarkdown = ""
			}
		}

		switch action.Type {
		case "create_note":
			normalized.Actions = append(normalized.Actions, action)
		case "update_note":
			if action.NoteID == "" {
				continue
			}
			if existing := updateActions[action.NoteID]; existing != nil {
				mergeAddActions(existing, action)
				continue
			}
			copy := action
			updateActions[action.NoteID] = &copy
			actionOrder = append(actionOrder, action.NoteID)
		}
	}

	for _, noteID := range actionOrder {
		if action := updateActions[noteID]; action != nil {
			normalized.Actions = append(normalized.Actions, *action)
		}
	}

	if normalized.Summary == "" {
		switch {
		case len(normalized.Actions) == 0:
			normalized.Summary = "Capture stored without KB changes."
		case len(normalized.Actions) == 1 && normalized.Actions[0].Type == "update_note":
			normalized.Summary = "Updated an existing note from the capture."
		case len(normalized.Actions) == 1 && normalized.Actions[0].Type == "create_note":
			normalized.Summary = "Created a new note from the capture."
		default:
			normalized.Summary = "Applied the capture to the knowledge base."
		}
	}

	return normalized, nil
}

func mergeAddActions(dst *AddAction, src AddAction) {
	dst.Reason = firstNonEmptyString(dst.Reason, src.Reason)
	dst.AliasesAdd = compactStrings(append(dst.AliasesAdd, src.AliasesAdd...))
	if len(src.TopicsSet) > 0 {
		dst.TopicsSet = compactStrings(src.TopicsSet)
	}
	dst.SummarySet = firstNonEmptyString(src.SummarySet, dst.SummarySet)
	dst.BodyMarkdown = firstNonEmptyString(dst.BodyMarkdown, src.BodyMarkdown)
	if src.BodyAppendMarkdown != "" {
		if dst.BodyAppendMarkdown == "" {
			dst.BodyAppendMarkdown = src.BodyAppendMarkdown
		} else if !strings.Contains(dst.BodyAppendMarkdown, src.BodyAppendMarkdown) {
			dst.BodyAppendMarkdown = strings.TrimRight(dst.BodyAppendMarkdown, "\n") + "\n\n" + src.BodyAppendMarkdown
		}
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func clampString(raw string, max int) string {
	raw = strings.TrimSpace(raw)
	if max <= 0 || len(raw) <= max {
		return raw
	}
	return strings.TrimSpace(raw[:max])
}

func validateAddPlan(plan *AddPlan, notes []*CanonicalNote) error {
	if plan == nil {
		return fmt.Errorf("empty add plan")
	}
	if len(plan.Actions) > 4 {
		return fmt.Errorf("plan includes too many actions")
	}
	if strings.TrimSpace(plan.Summary) == "" {
		return fmt.Errorf("plan summary is required")
	}

	noteIDs := make(map[string]struct{}, len(notes))
	noteTitles := make(map[string]struct{}, len(notes))
	for _, note := range notes {
		noteIDs[note.ID] = struct{}{}
		noteTitles[strings.ToLower(strings.TrimSpace(note.Title))] = struct{}{}
	}

	if len(plan.Actions) == 0 {
		return nil
	}

	for _, action := range plan.Actions {
		switch action.Type {
		case "create_note":
			if strings.TrimSpace(action.Title) == "" {
				return fmt.Errorf("create_note requires title")
			}
			if strings.TrimSpace(action.BodyMarkdown) == "" {
				return fmt.Errorf("create_note requires body_markdown")
			}
			if _, exists := noteTitles[strings.ToLower(strings.TrimSpace(action.Title))]; exists {
				return fmt.Errorf("create_note duplicates an existing title %q", action.Title)
			}
		case "update_note":
			if strings.TrimSpace(action.NoteID) == "" {
				return fmt.Errorf("update_note requires note_id")
			}
			if _, ok := noteIDs[action.NoteID]; !ok {
				return fmt.Errorf("update_note references unknown note_id %q", action.NoteID)
			}
			if strings.TrimSpace(action.BodyAppendMarkdown) == "" && strings.TrimSpace(action.SummarySet) == "" && len(action.AliasesAdd) == 0 && len(action.TopicsSet) == 0 {
				return fmt.Errorf("update_note requires at least one change")
			}
		default:
			return fmt.Errorf("unsupported action type %q", action.Type)
		}
	}

	return nil
}

func applyAddPlan(kbPath string, config *Config, capture *CaptureRecord, plan *AddPlan, notes []*CanonicalNote, reservedNoteIDs []string, now time.Time) (*AddResult, error) {
	originalsByID := make(map[string]*CanonicalNote, len(notes))
	notesByID := make(map[string]*CanonicalNote, len(notes))
	for _, note := range notes {
		if note == nil {
			continue
		}
		originalsByID[note.ID] = cloneCanonicalNote(note)
		notesByID[note.ID] = cloneCanonicalNote(note)
	}

	createIndex := 0
	changedNoteIDs := make([]string, 0, len(plan.Actions))
	changedSet := map[string]struct{}{}
	markChanged := func(noteID string) {
		if _, exists := changedSet[noteID]; exists {
			return
		}
		changedSet[noteID] = struct{}{}
		changedNoteIDs = append(changedNoteIDs, noteID)
	}
	result := &AddResult{
		Summary: strings.TrimSpace(plan.Summary),
	}
	if result.Summary == "" {
		result.Summary = "Capture applied."
	}

	for _, action := range plan.Actions {
		switch action.Type {
		case "create_note":
			if createIndex >= len(reservedNoteIDs) {
				return nil, fmt.Errorf("missing reserved note ID for create action")
			}
			note := &CanonicalNote{
				ID:               reservedNoteIDs[createIndex],
				Title:            strings.TrimSpace(action.Title),
				Aliases:          compactStrings(action.AliasesAdd),
				Summary:          strings.TrimSpace(action.SummarySet),
				Body:             strings.TrimSpace(action.BodyMarkdown),
				Topics:           compactStrings(action.TopicsSet),
				SourceCaptureIDs: []string{capture.ID},
				CreatedAt:        now,
				UpdatedAt:        now,
				Revision:         1,
			}
			if note.Title == "" {
				note.Title = inferNoteTitle(capture.RawContent)
			}
			if note.Body == "" {
				note.Body = strings.TrimSpace(capture.RawContent)
			}
			notesByID[note.ID] = note
			markChanged(note.ID)
			createIndex++
			result.Created = append(result.Created, note.Title)
		case "update_note":
			note := notesByID[action.NoteID]
			if note == nil {
				return nil, fmt.Errorf("note %s not found", action.NoteID)
			}

			changed := false
			note.Aliases = compactStrings(append(note.Aliases, action.AliasesAdd...))
			if len(action.AliasesAdd) > 0 {
				changed = true
			}
			if strings.TrimSpace(action.SummarySet) != "" {
				summary := strings.TrimSpace(action.SummarySet)
				if note.Summary != summary {
					note.Summary = summary
					changed = true
				}
			}
			if len(action.TopicsSet) > 0 {
				topics := compactStrings(action.TopicsSet)
				if strings.Join(note.Topics, "\x00") != strings.Join(topics, "\x00") {
					note.Topics = topics
					changed = true
				}
			}
			if chunk := strings.TrimSpace(action.BodyAppendMarkdown); chunk != "" {
				if !noteAlreadyContainsCapture(note, chunk) {
					if note.Body != "" {
						note.Body = strings.TrimRight(note.Body, "\n") + "\n\n" + chunk
					} else {
						note.Body = chunk
					}
					changed = true
				}
			}

			newCaptureIDs := compactStrings(append(note.SourceCaptureIDs, capture.ID))
			if strings.Join(note.SourceCaptureIDs, "\x00") != strings.Join(newCaptureIDs, "\x00") {
				note.SourceCaptureIDs = newCaptureIDs
				changed = true
			}
			if !changed {
				continue
			}
			note.UpdatedAt = now
			note.Revision++
			markChanged(note.ID)
			result.Updated = append(result.Updated, note.Title)
		}
	}

	changes := make([]canonicalNoteChange, 0, len(changedNoteIDs))
	for _, noteID := range changedNoteIDs {
		changes = append(changes, canonicalNoteChange{
			Before: cloneCanonicalNote(originalsByID[noteID]),
			After:  cloneCanonicalNote(notesByID[noteID]),
		})
	}
	if err := persistCanonicalNoteChanges(kbPath, config, changes); err != nil {
		return nil, err
	}
	for _, change := range changes {
		if change.After == nil || change.After.MaterializedPath == "" {
			continue
		}
		result.Materialized = append(result.Materialized, change.After.MaterializedPath)
	}

	return result, nil
}

func saveAndIndexCanonicalNote(kbPath string, config *Config, note *CanonicalNote) error {
	before, err := loadCanonicalNoteRecord(kbPath, note.ID)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	after := cloneCanonicalNote(note)
	if err := persistCanonicalNoteChanges(kbPath, config, []canonicalNoteChange{{
		Before: cloneCanonicalNote(before),
		After:  after,
	}}); err != nil {
		return err
	}
	*note = *after
	return nil
}

type reservedAddIdentifiersResult struct {
	CaptureID   string
	OperationID string
	NoteIDs     []string
}

func reserveAddIdentifiers(state *KBState, plan *AddPlan, now time.Time) reservedAddIdentifiersResult {
	result := reservedAddIdentifiersResult{
		CaptureID:   nextCaptureID(state, now),
		OperationID: nextOperationID(state, now),
	}

	for _, action := range plan.Actions {
		if action.Type == "create_note" {
			result.NoteIDs = append(result.NoteIDs, nextNoteID(state, now))
		}
	}

	return result
}

func newPendingOperation(id, captureID, provider, model string, plan *AddPlan, now time.Time) *AppliedOperation {
	summary := ""
	actions := []AddAction(nil)
	if plan != nil {
		summary = strings.TrimSpace(plan.Summary)
		actions = append(actions, plan.Actions...)
	}

	return &AppliedOperation{
		ID:        id,
		CaptureID: captureID,
		Status:    OperationStatusPending,
		Provider:  provider,
		Model:     model,
		Summary:   summary,
		Actions:   actions,
		PlannedAt: now,
		UpdatedAt: now,
	}
}

type canonicalNoteChange struct {
	Before *CanonicalNote
	After  *CanonicalNote
}

func cloneCanonicalNote(note *CanonicalNote) *CanonicalNote {
	if note == nil {
		return nil
	}

	copy := *note
	copy.Aliases = append([]string(nil), note.Aliases...)
	copy.Topics = append([]string(nil), note.Topics...)
	copy.SourceCaptureIDs = append([]string(nil), note.SourceCaptureIDs...)
	return &copy
}

func persistCanonicalNoteChanges(kbPath string, config *Config, changes []canonicalNoteChange) error {
	if len(changes) == 0 {
		return nil
	}

	applied := make([]canonicalNoteChange, 0, len(changes))
	for _, change := range changes {
		if change.After == nil {
			continue
		}
		if err := materializeCanonicalNote(kbPath, change.After); err != nil {
			if rollbackErr := rollbackCanonicalNoteChanges(kbPath, config, applied); rollbackErr != nil {
				return fmt.Errorf("failed to materialize note: %w (rollback failed: %v)", err, rollbackErr)
			}
			return fmt.Errorf("failed to materialize note: %w", err)
		}
		currentChange := canonicalNoteChange{
			Before: cloneCanonicalNote(change.Before),
			After:  cloneCanonicalNote(change.After),
		}
		if err := saveCanonicalNoteRecord(kbPath, change.After); err != nil {
			rollbackSet := append(append([]canonicalNoteChange(nil), applied...), currentChange)
			if rollbackErr := rollbackCanonicalNoteChanges(kbPath, config, rollbackSet); rollbackErr != nil {
				return fmt.Errorf("failed to save note record: %w (rollback failed: %v)", err, rollbackErr)
			}
			return fmt.Errorf("failed to save note record: %w", err)
		}
		applied = append(applied, currentChange)
	}

	if err := rebuildIndexIfEnabled(kbPath, config); err != nil {
		if rollbackErr := rollbackCanonicalNoteChanges(kbPath, config, applied); rollbackErr != nil {
			return fmt.Errorf("failed to rebuild index: %w (rollback failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("failed to rebuild index: %w", err)
	}

	return nil
}

func rollbackCanonicalNoteChanges(kbPath string, config *Config, changes []canonicalNoteChange) error {
	var rollbackErrors []string
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		switch {
		case change.Before == nil:
			if err := deleteCanonicalNoteRecord(kbPath, change.After); err != nil {
				rollbackErrors = append(rollbackErrors, err.Error())
			}
		default:
			restored := cloneCanonicalNote(change.Before)
			if change.After != nil && change.After.MaterializedPath != "" {
				restored.MaterializedPath = change.After.MaterializedPath
			}
			if err := materializeCanonicalNote(kbPath, restored); err != nil {
				rollbackErrors = append(rollbackErrors, err.Error())
				continue
			}
			if err := saveCanonicalNoteRecord(kbPath, restored); err != nil {
				rollbackErrors = append(rollbackErrors, err.Error())
			}
		}
	}

	if err := rebuildIndexIfEnabled(kbPath, config); err != nil {
		rollbackErrors = append(rollbackErrors, err.Error())
	}
	if len(rollbackErrors) > 0 {
		return fmt.Errorf(strings.Join(rollbackErrors, "; "))
	}
	return nil
}

func rebuildIndexIfEnabled(kbPath string, config *Config) error {
	if !shouldAutoUpdateIndex(config) {
		return nil
	}

	index, err := RebuildIndex(kbPath)
	if err != nil {
		return err
	}
	return SaveIndex(index, kbPath)
}

func inferNoteTitle(content string) string {
	env := parseCaptureEnvelope(content)
	lines := strings.Split(strings.ReplaceAll(env.Body, "\r\n", "\n"), "\n")
	for _, rawLine := range lines {
		line := cleanPotentialTitleLine(rawLine)
		if line == "" {
			continue
		}
		return normalizeTitle(line)
	}
	if sourceTitle := cleanPotentialTitleLine(env.SourceTitle); sourceTitle != "" {
		return normalizeTitle(sourceTitle)
	}

	return "Captured Note"
}

func cleanPotentialTitleLine(raw string) string {
	line := strings.TrimSpace(raw)
	line = strings.TrimLeft(line, "#-*0123456789. ")
	line = strings.TrimSpace(line)
	if line == "" || line == "```" {
		return ""
	}
	lowerLine := strings.ToLower(line)
	if strings.HasPrefix(lowerLine, "source file:") || strings.HasPrefix(lowerLine, "source url:") {
		return ""
	}
	if strings.HasPrefix(lowerLine, "source title:") {
		line = strings.TrimSpace(line[len("source title:"):])
		lowerLine = strings.ToLower(line)
	}

	for _, prefix := range []string{
		"remember ",
		"how to ",
		"tip: ",
		"note: ",
		"todo: ",
		"use ",
	} {
		if strings.HasPrefix(lowerLine, prefix) {
			line = strings.TrimSpace(line[len(prefix):])
			lowerLine = strings.ToLower(line)
			break
		}
	}

	for _, sep := range []string{". ", ": ", "\n"} {
		if idx := strings.Index(line, sep); idx > 12 {
			line = strings.TrimSpace(line[:idx])
			break
		}
	}

	line = strings.Trim(line, " .:-")
	if len(line) > 90 {
		line = strings.TrimSpace(line[:90])
	}

	return line
}

func normalizeTitle(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "Captured Note"
	}

	if strings.EqualFold(raw, strings.ToLower(raw)) || looksLikeSlug(strings.ReplaceAll(raw, " ", "-")) {
		return smartTitleCase(raw)
	}

	return strings.ToUpper(raw[:1]) + raw[1:]
}

func smartTitleCase(raw string) string {
	raw = strings.ReplaceAll(raw, "/", " ")
	words := strings.Fields(strings.ReplaceAll(strings.ReplaceAll(raw, "-", " "), "_", " "))
	if len(words) == 0 {
		return ""
	}

	smallWords := map[string]struct{}{
		"and": {}, "or": {}, "of": {}, "on": {}, "to": {}, "in": {}, "for": {}, "the": {}, "a": {}, "an": {},
	}
	specialWords := map[string]string{
		"macos":    "macOS",
		"ios":      "iOS",
		"openai":   "OpenAI",
		"claude":   "Claude",
		"ssh":      "SSH",
		"tcp":      "TCP",
		"udp":      "UDP",
		"api":      "API",
		"cli":      "CLI",
		"url":      "URL",
		"kb":       "KB",
		"postgres": "Postgres",
	}

	for i, word := range words {
		lower := strings.ToLower(word)
		if replacement, ok := specialWords[lower]; ok {
			words[i] = replacement
			continue
		}
		if i > 0 {
			if _, ok := smallWords[lower]; ok {
				words[i] = lower
				continue
			}
		}
		if isAllUpperToken(word) {
			words[i] = word
			continue
		}
		words[i] = strings.ToUpper(lower[:1]) + lower[1:]
	}

	return strings.Join(words, " ")
}

func isAllUpperToken(raw string) bool {
	hasLetter := false
	for _, r := range raw {
		if r >= 'A' && r <= 'Z' {
			hasLetter = true
			continue
		}
		if r >= 'a' && r <= 'z' {
			return false
		}
	}
	return hasLetter
}

func inferSummaryFromContent(content string) string {
	return inferSummaryFromContentWithTitle(content, inferNoteTitle(content))
}

func inferTopicsFromContent(content string) []string {
	lower := strings.ToLower(content)
	topics := []string{}
	for _, candidate := range []string{
		"macos", "linux", "docker", "ssh", "networking", "git", "go", "openai", "claude",
		"postgres", "postgresql", "mysql", "kubernetes", "terraform", "javascript", "typescript", "python",
	} {
		if strings.Contains(lower, candidate) {
			switch candidate {
			case "postgresql":
				topics = append(topics, "postgres")
			default:
				topics = append(topics, candidate)
			}
		}
	}
	return compactStrings(topics)
}

func normalizeCaptureContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.TrimSpace(content)
}

func printAddDryRun(plan *AddPlan, asJSON bool) {
	if asJSON {
		data, _ := json.MarshalIndent(plan, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println(Header("Dry run:"))
	fmt.Printf("  Summary: %s\n", Cyan(strings.TrimSpace(plan.Summary)))
	for i, action := range plan.Actions {
		fmt.Printf("  %d. %s", i+1, action.Type)
		switch action.Type {
		case "create_note":
			fmt.Printf(" %s\n", Bold(action.Title))
		case "update_note":
			fmt.Printf(" %s\n", Bold(action.NoteID))
		default:
			fmt.Println()
		}
	}
}

func printAddResult(result *AddResult, asJSON bool) {
	if asJSON {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println("Captured.")
	if result.Summary != "" {
		fmt.Printf("Summary: %s\n", result.Summary)
	}
	for _, title := range result.Updated {
		fmt.Printf("Updated: %s\n", title)
	}
	for _, title := range result.Created {
		fmt.Printf("Created: %s\n", title)
	}
}
