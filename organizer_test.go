package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlanAddWithOpenAIParsesStructuredJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"resp_123",
			"object":"response",
			"status":"completed",
			"model":"gpt-test",
			"output":[
				{
					"id":"msg_123",
					"type":"message",
					"role":"assistant",
					"status":"completed",
					"content":[
						{
							"type":"output_text",
							"text":"{\"summary\":\"Merged into an existing note.\",\"actions\":[{\"type\":\"update_note\",\"note_id\":\"note_1\",\"reason\":\"related topic\",\"body_append_markdown\":\"Added detail.\",\"aliases_add\":[\"ports mac\"]}]}",
							"annotations":[]
						}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", server.URL)
	t.Setenv("OPENAI_MODEL", "gpt-test")

	plan, model, err := planAddWithOpenAI("remember lsof on macos", []*CanonicalNote{
		{
			ID:      "note_1",
			Title:   "Inspect Open Ports on macOS",
			Summary: "Existing note.",
		},
	})
	if err != nil {
		t.Fatalf("planAddWithOpenAI failed: %v", err)
	}

	if model != "gpt-test" {
		t.Fatalf("expected model gpt-test, got %s", model)
	}
	if plan.Summary != "Merged into an existing note." {
		t.Fatalf("unexpected plan summary: %s", plan.Summary)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("expected one action, got %d", len(plan.Actions))
	}
	if plan.Actions[0].Type != "update_note" || plan.Actions[0].NoteID != "note_1" {
		t.Fatalf("unexpected action: %+v", plan.Actions[0])
	}
}

func TestPlanAddHeuristicallyUpdatesStrongMatch(t *testing.T) {
	plan, ok := planAddHeuristically("remember lsof -iTCP -sTCP:LISTEN on macos", []*CanonicalNote{
		{
			ID:      "note_1",
			Title:   "Inspect Open Ports on macOS",
			Summary: "Check listening TCP ports.",
			Topics:  []string{"macos", "networking"},
			Body:    "Use lsof -iTCP -sTCP:LISTEN to inspect listening ports on macOS.",
		},
	})
	if !ok {
		t.Fatal("expected heuristic planner to match existing note")
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("expected one action, got %d", len(plan.Actions))
	}
	if plan.Actions[0].Type != "update_note" || plan.Actions[0].NoteID != "note_1" {
		t.Fatalf("unexpected heuristic action: %+v", plan.Actions[0])
	}
}

func TestNormalizeAddPlanConvertsDuplicateCreateToUpdate(t *testing.T) {
	plan, err := normalizeAddPlan(&AddPlan{
		Summary: "update existing knowledge",
		Actions: []AddAction{
			{
				Type:         "create_note",
				Title:        "Inspect Open Ports on macOS",
				BodyMarkdown: "Use lsof.",
			},
		},
	}, []*CanonicalNote{
		{
			ID:    "note_1",
			Title: "Inspect Open Ports on macOS",
			Body:  "Existing body.",
		},
	})
	if err != nil {
		t.Fatalf("normalizeAddPlan failed: %v", err)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("expected one normalized action, got %d", len(plan.Actions))
	}
	if plan.Actions[0].Type != "update_note" || plan.Actions[0].NoteID != "note_1" {
		t.Fatalf("expected duplicate create to become update_note, got %+v", plan.Actions[0])
	}
	if plan.Actions[0].BodyAppendMarkdown != "Use lsof." {
		t.Fatalf("expected body append markdown to be preserved, got %q", plan.Actions[0].BodyAppendMarkdown)
	}
}
