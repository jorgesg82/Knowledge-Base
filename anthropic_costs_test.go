package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchAnthropicSpendSummary(t *testing.T) {
	now := time.Date(2026, time.March, 6, 15, 4, 5, 0, time.UTC).In(time.Local)
	totalStart := anthropicCostsMinimumStartTime.UTC().Format(time.RFC3339)
	last30Start := now.AddDate(0, 0, -30).UTC().Format(time.RFC3339)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UTC().Format(time.RFC3339)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "test-admin-key" {
			t.Fatalf("unexpected x-api-key header: %q", got)
		}
		if got := r.URL.Query().Get("bucket_width"); got != "1d" {
			t.Fatalf("unexpected bucket width: %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "31" {
			t.Fatalf("unexpected limit: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")

		switch startTime := r.URL.Query().Get("starting_at"); startTime {
		case totalStart:
			if r.URL.Query().Get("page") == "" {
				fmt.Fprint(w, `{"data":[{"results":[{"amount":{"value":"125","currency":"usd"}}]}],"has_more":true,"next_page":"page-2"}`)
				return
			}
			if r.URL.Query().Get("page") == "page-2" {
				fmt.Fprint(w, `{"data":[{"results":[{"amount":{"value":"375","currency":"usd"}}]}],"has_more":false,"next_page":""}`)
				return
			}
			t.Fatalf("unexpected total page: %q", r.URL.Query().Get("page"))
		case last30Start:
			fmt.Fprint(w, `{"data":[{"results":[{"amount":{"value":"200","currency":"usd"}}]}],"has_more":false,"next_page":""}`)
		case dayStart:
			fmt.Fprint(w, `{"data":[{"results":[{"amount":{"value":"25","currency":"usd"}}]}],"has_more":false,"next_page":""}`)
		default:
			t.Fatalf("unexpected starting_at query: %s", startTime)
		}
	}))
	defer server.Close()

	t.Setenv("ANTHROPIC_ADMIN_KEY", "test-admin-key")
	t.Setenv("ANTHROPIC_ADMIN_API_KEY", "")
	t.Setenv("ANTHROPIC_ADMIN_BASE_URL", server.URL)
	t.Setenv("ANTHROPIC_BASE_URL", "")

	summary, err := FetchAnthropicSpendSummary(now)
	if err != nil {
		t.Fatalf("FetchAnthropicSpendSummary failed: %v", err)
	}

	if summary.Currency != "usd" {
		t.Fatalf("expected usd currency, got %s", summary.Currency)
	}
	if summary.Total != 5.0 {
		t.Fatalf("expected total 5.0, got %.2f", summary.Total)
	}
	if summary.Last30Days != 2.0 {
		t.Fatalf("expected last30 2.0, got %.2f", summary.Last30Days)
	}
	if summary.Today != 0.25 {
		t.Fatalf("expected today 0.25, got %.2f", summary.Today)
	}
}

func TestFetchAnthropicSpendSummaryRequiresAdminKey(t *testing.T) {
	t.Setenv("ANTHROPIC_ADMIN_KEY", "")
	t.Setenv("ANTHROPIC_ADMIN_API_KEY", "")

	_, err := FetchAnthropicSpendSummary(time.Now())
	if err == nil {
		t.Fatal("expected missing admin key error")
	}
	if err != ErrAnthropicAdminKeyNotSet {
		t.Fatalf("expected ErrAnthropicAdminKeyNotSet, got %v", err)
	}
}
