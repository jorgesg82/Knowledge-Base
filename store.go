package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	capturesDirName       = "captures"
	notesDirName          = "notes"
	opsDirName            = "ops"
	stateFileName         = "state.json"
	notesManifestFileName = "notes_manifest.json"
)

type CaptureSource string

const (
	CaptureSourceCLI       CaptureSource = "cli"
	CaptureSourceEditor    CaptureSource = "editor"
	CaptureSourceStdin     CaptureSource = "stdin"
	CaptureSourceFile      CaptureSource = "file"
	CaptureSourceURL       CaptureSource = "url"
	CaptureSourceClipboard CaptureSource = "clipboard"
)

type CaptureRecord struct {
	ID                string        `json:"id"`
	CreatedAt         time.Time     `json:"created_at"`
	Source            CaptureSource `json:"source"`
	RawContent        string        `json:"raw_content"`
	NormalizedContent string        `json:"normalized_content"`
	Provider          string        `json:"provider,omitempty"`
	Model             string        `json:"model,omitempty"`
	OperationID       string        `json:"operation_id,omitempty"`
}

type CanonicalNote struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Aliases          []string  `json:"aliases,omitempty"`
	Summary          string    `json:"summary,omitempty"`
	Body             string    `json:"body"`
	Topics           []string  `json:"topics,omitempty"`
	SourceCaptureIDs []string  `json:"source_capture_ids,omitempty"`
	MaterializedPath string    `json:"materialized_path"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Revision         int       `json:"revision"`
}

type CanonicalNoteManifestEntry struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Aliases          []string  `json:"aliases,omitempty"`
	Summary          string    `json:"summary,omitempty"`
	Topics           []string  `json:"topics,omitempty"`
	SourceCaptureIDs []string  `json:"source_capture_ids,omitempty"`
	MaterializedPath string    `json:"materialized_path"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Revision         int       `json:"revision"`
}

type AddPlan struct {
	Summary string      `json:"summary"`
	Actions []AddAction `json:"actions"`
}

type AddAction struct {
	Type               string   `json:"type"`
	NoteID             string   `json:"note_id,omitempty"`
	Title              string   `json:"title,omitempty"`
	Reason             string   `json:"reason,omitempty"`
	AliasesAdd         []string `json:"aliases_add,omitempty"`
	TopicsSet          []string `json:"topics_set,omitempty"`
	SummarySet         string   `json:"summary_set,omitempty"`
	BodyMarkdown       string   `json:"body_markdown,omitempty"`
	BodyAppendMarkdown string   `json:"body_append_markdown,omitempty"`
}

type AppliedOperation struct {
	ID        string          `json:"id"`
	CaptureID string          `json:"capture_id"`
	Status    OperationStatus `json:"status"`
	Provider  string          `json:"provider,omitempty"`
	Model     string          `json:"model,omitempty"`
	Summary   string          `json:"summary"`
	Actions   []AddAction     `json:"actions"`
	Error     string          `json:"error,omitempty"`
	PlannedAt time.Time       `json:"planned_at"`
	AppliedAt time.Time       `json:"applied_at,omitempty"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type OperationStatus string

const (
	OperationStatusPending OperationStatus = "pending"
	OperationStatusApplied OperationStatus = "applied"
	OperationStatusFailed  OperationStatus = "failed"
)

type KBState struct {
	Version          int       `json:"version"`
	CreatedAt        time.Time `json:"created_at"`
	LastCaptureSeq   int       `json:"last_capture_seq"`
	LastNoteSeq      int       `json:"last_note_seq"`
	LastOperationSeq int       `json:"last_operation_seq"`
}

type materializedNoteFrontmatter struct {
	ID             string    `yaml:"id"`
	Title          string    `yaml:"title"`
	Aliases        []string  `yaml:"aliases,omitempty"`
	Summary        string    `yaml:"summary,omitempty"`
	Category       string    `yaml:"category"`
	Topics         []string  `yaml:"topics,omitempty"`
	SourceCaptures []string  `yaml:"source_captures,omitempty"`
	Created        time.Time `yaml:"created"`
	Updated        time.Time `yaml:"updated"`
}

func ensureStoreLayout(kbPath string) error {
	dirs := []string{
		filepath.Join(kbPath, ".kb", capturesDirName),
		filepath.Join(kbPath, ".kb", notesDirName),
		filepath.Join(kbPath, ".kb", opsDirName),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	return nil
}

func loadOrInitKBState(kbPath string) (*KBState, error) {
	if err := ensureStoreLayout(kbPath); err != nil {
		return nil, err
	}

	statePath := filepath.Join(kbPath, ".kb", stateFileName)
	data, err := os.ReadFile(statePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read state: %w", err)
		}

		state := &KBState{
			Version:   2,
			CreatedAt: time.Now().UTC(),
		}
		if err := saveKBState(kbPath, state); err != nil {
			return nil, err
		}
		return state, nil
	}

	var state KBState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state: %w", err)
	}
	if state.Version == 0 {
		state.Version = 2
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = time.Now().UTC()
	}

	return &state, nil
}

func saveKBState(kbPath string, state *KBState) error {
	statePath := filepath.Join(kbPath, ".kb", stateFileName)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}
	if err := writeFileAtomically(statePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}
	return nil
}

func nextCaptureID(state *KBState, now time.Time) string {
	state.LastCaptureSeq++
	return fmt.Sprintf("cap_%s_%04d_%s", now.UTC().Format("20060102T150405Z"), state.LastCaptureSeq, randomIDSuffix())
}

func nextNoteID(state *KBState, now time.Time) string {
	state.LastNoteSeq++
	return fmt.Sprintf("note_%s_%04d_%s", now.UTC().Format("20060102T150405Z"), state.LastNoteSeq, randomIDSuffix())
}

func nextOperationID(state *KBState, now time.Time) string {
	state.LastOperationSeq++
	return fmt.Sprintf("op_%s_%04d_%s", now.UTC().Format("20060102T150405Z"), state.LastOperationSeq, randomIDSuffix())
}

func randomIDSuffix() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%08x", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}

func saveCaptureRecord(kbPath string, record *CaptureRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal capture: %w", err)
	}

	capturePath := filepath.Join(kbPath, ".kb", capturesDirName, record.ID+".json")
	if err := writeFileAtomically(capturePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write capture: %w", err)
	}

	return nil
}

func saveAppliedOperation(kbPath string, op *AppliedOperation) error {
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal operation: %w", err)
	}

	opPath := filepath.Join(kbPath, ".kb", opsDirName, op.ID+".json")
	if err := writeFileAtomically(opPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write operation: %w", err)
	}

	return nil
}

func loadCanonicalNotes(kbPath string) ([]*CanonicalNote, error) {
	return loadCanonicalNotesFromDisk(kbPath)
}

func loadCanonicalNoteRecord(kbPath, noteID string) (*CanonicalNote, error) {
	noteID = strings.TrimSpace(noteID)
	if noteID == "" {
		return nil, os.ErrNotExist
	}

	notePath := filepath.Join(kbPath, ".kb", notesDirName, noteID+".json")
	data, err := os.ReadFile(notePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("failed to read note %s: %w", noteID, err)
	}

	var note CanonicalNote
	if err := json.Unmarshal(data, &note); err != nil {
		return nil, fmt.Errorf("failed to parse note %s: %w", noteID, err)
	}

	return &note, nil
}

func loadCanonicalNotesFromDisk(kbPath string) ([]*CanonicalNote, error) {
	notesDir := filepath.Join(kbPath, ".kb", notesDirName)
	if _, err := os.Stat(notesDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(notesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read notes directory: %w", err)
	}

	var notes []*CanonicalNote
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(notesDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read note %s: %w", entry.Name(), err)
		}

		var note CanonicalNote
		if err := json.Unmarshal(data, &note); err != nil {
			return nil, fmt.Errorf("failed to parse note %s: %w", entry.Name(), err)
		}

		notes = append(notes, &note)
	}

	sort.Slice(notes, func(i, j int) bool {
		return notes[i].CreatedAt.Before(notes[j].CreatedAt)
	})

	return notes, nil
}

func loadCanonicalNoteManifest(kbPath string) ([]*CanonicalNoteManifestEntry, error) {
	notes, ok, err := loadCanonicalNoteManifestFile(kbPath)
	if err == nil && ok {
		return notes, nil
	}

	fullNotes, err := loadCanonicalNotesFromDisk(kbPath)
	if err != nil {
		return nil, err
	}
	notes = canonicalNoteManifestEntriesFromNotes(fullNotes)
	if err := saveCanonicalNotesManifest(kbPath, notes); err != nil {
		return nil, err
	}
	return notes, nil
}

func loadCanonicalNoteManifestFile(kbPath string) ([]*CanonicalNoteManifestEntry, bool, error) {
	manifestPath := filepath.Join(kbPath, ".kb", notesManifestFileName)
	manifestInfo, err := os.Stat(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to stat notes manifest: %w", err)
	}

	notesDir := filepath.Join(kbPath, ".kb", notesDirName)
	entries, err := os.ReadDir(notesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to read notes directory: %w", err)
	}

	jsonFiles := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		jsonFiles++
		info, err := entry.Info()
		if err != nil {
			return nil, false, fmt.Errorf("failed to stat note %s: %w", entry.Name(), err)
		}
		if info.ModTime().After(manifestInfo.ModTime()) {
			return nil, false, nil
		}
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read notes manifest: %w", err)
	}

	var notes []*CanonicalNoteManifestEntry
	if err := json.Unmarshal(data, &notes); err != nil {
		return nil, false, fmt.Errorf("failed to parse notes manifest: %w", err)
	}
	if len(notes) != jsonFiles {
		return nil, false, nil
	}

	sort.Slice(notes, func(i, j int) bool {
		return notes[i].CreatedAt.Before(notes[j].CreatedAt)
	})

	return notes, true, nil
}

func saveCanonicalNotesManifest(kbPath string, notes []*CanonicalNoteManifestEntry) error {
	manifestPath := filepath.Join(kbPath, ".kb", notesManifestFileName)
	data, err := json.MarshalIndent(notes, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal notes manifest: %w", err)
	}
	if err := writeFileAtomically(manifestPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write notes manifest: %w", err)
	}
	return nil
}

func canonicalNoteManifestEntriesFromNotes(notes []*CanonicalNote) []*CanonicalNoteManifestEntry {
	result := make([]*CanonicalNoteManifestEntry, 0, len(notes))
	for _, note := range notes {
		if note == nil {
			continue
		}
		result = append(result, canonicalNoteManifestEntryFromNote(note))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result
}

func canonicalNoteManifestEntryFromNote(note *CanonicalNote) *CanonicalNoteManifestEntry {
	if note == nil {
		return nil
	}

	return &CanonicalNoteManifestEntry{
		ID:               note.ID,
		Title:            note.Title,
		Aliases:          compactStrings(note.Aliases),
		Summary:          strings.TrimSpace(note.Summary),
		Topics:           compactStrings(note.Topics),
		SourceCaptureIDs: compactStrings(note.SourceCaptureIDs),
		MaterializedPath: note.MaterializedPath,
		CreatedAt:        note.CreatedAt,
		UpdatedAt:        note.UpdatedAt,
		Revision:         note.Revision,
	}
}

func upsertCanonicalNoteManifest(kbPath string, note *CanonicalNote) error {
	if note == nil {
		return nil
	}

	notes, ok, err := loadCanonicalNoteManifestFile(kbPath)
	if err != nil {
		return err
	}
	if !ok {
		fullNotes, err := loadCanonicalNotesFromDisk(kbPath)
		if err != nil {
			return err
		}
		notes = canonicalNoteManifestEntriesFromNotes(fullNotes)
	}

	replaced := false
	for i, existing := range notes {
		if existing != nil && existing.ID == note.ID {
			copy := *canonicalNoteManifestEntryFromNote(note)
			notes[i] = &copy
			replaced = true
			break
		}
	}
	if !replaced {
		copy := *canonicalNoteManifestEntryFromNote(note)
		notes = append(notes, &copy)
	}

	sort.Slice(notes, func(i, j int) bool {
		return notes[i].CreatedAt.Before(notes[j].CreatedAt)
	})
	return saveCanonicalNotesManifest(kbPath, notes)
}

func removeCanonicalNoteFromManifest(kbPath, noteID string) error {
	if strings.TrimSpace(noteID) == "" {
		return nil
	}

	notes, ok, err := loadCanonicalNoteManifestFile(kbPath)
	if err != nil {
		return err
	}
	if !ok {
		fullNotes, err := loadCanonicalNotesFromDisk(kbPath)
		if err != nil {
			return err
		}
		notes = canonicalNoteManifestEntriesFromNotes(fullNotes)
	}

	filtered := notes[:0]
	for _, note := range notes {
		if note == nil || note.ID == noteID {
			continue
		}
		filtered = append(filtered, note)
	}

	return saveCanonicalNotesManifest(kbPath, filtered)
}

func saveCanonicalNoteRecord(kbPath string, note *CanonicalNote) error {
	data, err := json.MarshalIndent(note, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal note: %w", err)
	}

	notePath := filepath.Join(kbPath, ".kb", notesDirName, note.ID+".json")
	if err := writeFileAtomically(notePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write note: %w", err)
	}

	return upsertCanonicalNoteManifest(kbPath, note)
}

func deleteCanonicalNoteRecord(kbPath string, note *CanonicalNote) error {
	if note == nil {
		return nil
	}

	notePath := filepath.Join(kbPath, ".kb", notesDirName, note.ID+".json")
	if err := os.Remove(notePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove note record: %w", err)
	}
	if note.MaterializedPath != "" {
		if err := os.Remove(filepath.Join(kbPath, note.MaterializedPath)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove materialized note: %w", err)
		}
	}
	if err := removeCanonicalNoteFromManifest(kbPath, note.ID); err != nil {
		return err
	}

	return nil
}

func materializeCanonicalNote(kbPath string, note *CanonicalNote) error {
	relPath := buildMaterializedNotePath(note)
	if relPath == "" {
		relPath = filepath.Join("entries", note.ID+".md")
	}

	previousPath := note.MaterializedPath
	note.MaterializedPath = relPath
	frontmatter := materializedNoteFrontmatter{
		ID:             note.ID,
		Title:          note.Title,
		Aliases:        compactStrings(note.Aliases),
		Summary:        strings.TrimSpace(note.Summary),
		Category:       deriveNoteCategory(note),
		Topics:         compactStrings(note.Topics),
		SourceCaptures: compactStrings(note.SourceCaptureIDs),
		Created:        note.CreatedAt,
		Updated:        note.UpdatedAt,
	}

	meta, err := yaml.Marshal(frontmatter)
	if err != nil {
		return fmt.Errorf("failed to marshal note frontmatter: %w", err)
	}

	body := strings.TrimSpace(note.Body)
	if body == "" {
		body = "# " + note.Title
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(meta)
	buf.WriteString("---\n\n")
	if !strings.HasPrefix(body, "#") {
		buf.WriteString("# ")
		buf.WriteString(note.Title)
		buf.WriteString("\n\n")
	}
	buf.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		buf.WriteString("\n")
	}

	absPath := filepath.Join(kbPath, relPath)
	if err := writeFileAtomically(absPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write materialized note: %w", err)
	}
	if previousPath != "" && previousPath != relPath {
		_ = os.Remove(filepath.Join(kbPath, previousPath))
	}

	return nil
}

func buildMaterializedNotePath(note *CanonicalNote) string {
	titleSlug := slugifyTitle(note.Title)
	if titleSlug == "" {
		titleSlug = note.ID
	}

	filename := titleSlug + ".md"
	shortID := shortStableID(note.ID)
	if shortID != "" {
		filename = titleSlug + "-" + shortID + ".md"
	}

	topic := ""
	if len(note.Topics) > 0 {
		topic = slugifyTitle(note.Topics[0])
	}
	if topic == "" {
		return filepath.Join("entries", filename)
	}

	return filepath.Join("entries", topic, filename)
}

func deriveNoteCategory(note *CanonicalNote) string {
	if note == nil {
		return "knowledge"
	}

	return deriveNoteCategoryFromTopics(note.Topics)
}

func deriveNoteCategoryFromTopics(topics []string) string {
	if len(topics) == 0 {
		return "knowledge"
	}

	category := slugifyTitle(topics[0])
	if category == "" {
		return "knowledge"
	}

	return category
}

func shortStableID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if len(trimmed) <= 8 {
		return trimmed
	}

	return trimmed[len(trimmed)-8:]
}

func slugifyTitle(raw string) string {
	return GenerateID("", raw)
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}

	return result
}

func syncCanonicalNoteFromEntry(kbPath string, entry *Entry) error {
	if entry == nil {
		return nil
	}

	notes, err := loadCanonicalNotes(kbPath)
	if err != nil {
		return err
	}

	relPath, err := filepath.Rel(kbPath, entry.FilePath)
	if err != nil {
		return err
	}
	relPath = filepath.ToSlash(relPath)

	for _, note := range notes {
		if filepath.ToSlash(note.MaterializedPath) != relPath {
			continue
		}

		note.Title = strings.TrimSpace(entry.Metadata.Title)
		note.Aliases = compactStrings(entry.Metadata.Aliases)
		note.Summary = strings.TrimSpace(entry.Metadata.Summary)
		note.Body = strings.TrimSpace(entry.Content)
		note.Topics = compactStrings(entry.Metadata.Topics)
		if !entry.Metadata.Updated.IsZero() {
			note.UpdatedAt = entry.Metadata.Updated
		} else {
			note.UpdatedAt = time.Now().UTC()
		}
		if !entry.Metadata.Created.IsZero() && note.CreatedAt.IsZero() {
			note.CreatedAt = entry.Metadata.Created
		}
		if len(entry.Metadata.SourceCaptures) > 0 {
			note.SourceCaptureIDs = compactStrings(entry.Metadata.SourceCaptures)
		}
		note.Revision++

		return saveCanonicalNoteRecord(kbPath, note)
	}

	return nil
}

func removeCanonicalNoteByMaterializedPath(kbPath, entryPath string) error {
	notes, err := loadCanonicalNotes(kbPath)
	if err != nil {
		return err
	}

	relPath, err := filepath.Rel(kbPath, entryPath)
	if err != nil {
		return err
	}
	relPath = filepath.ToSlash(relPath)

	for _, note := range notes {
		if filepath.ToSlash(note.MaterializedPath) != relPath {
			continue
		}

		notePath := filepath.Join(kbPath, ".kb", notesDirName, note.ID+".json")
		if err := os.Remove(notePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove note record: %w", err)
		}
		return removeCanonicalNoteFromManifest(kbPath, note.ID)
	}

	return nil
}
