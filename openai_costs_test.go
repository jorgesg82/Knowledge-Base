package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchOpenAISpendSummary(t *testing.T) {
	now := time.Date(2026, time.March, 5, 15, 4, 5, 0, time.UTC).In(time.Local)
	last30Start := now.AddDate(0, 0, -30).Unix()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/organization/costs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer test-admin-key" {
			t.Fatalf("unexpected authorization header: %q", got)
		}

		startTime := r.URL.Query().Get("start_time")
		page := r.URL.Query().Get("page")

		w.Header().Set("Content-Type", "application/json")

		switch {
		case startTime == "0" && page == "":
			fmt.Fprint(w, `{"data":[{"results":[{"amount":{"value":5.00,"currency":"usd"}}]}],"has_more":true,"next_page":"page-2"}`)
		case startTime == "0" && page == "page-2":
			fmt.Fprint(w, `{"data":[{"results":[{"amount":{"value":7.00,"currency":"usd"}}]}],"has_more":false,"next_page":""}`)
		case startTime == fmt.Sprintf("%d", last30Start):
			fmt.Fprint(w, `{"data":[{"results":[{"amount":{"value":3.50,"currency":"usd"}}]}],"has_more":false,"next_page":""}`)
		case startTime == fmt.Sprintf("%d", dayStart):
			fmt.Fprint(w, `{"data":[{"results":[{"amount":{"value":0.25,"currency":"usd"}}]}],"has_more":false,"next_page":""}`)
		default:
			t.Fatalf("unexpected start_time=%s page=%s", startTime, page)
		}
	}))
	defer server.Close()

	t.Setenv("OPENAI_ADMIN_KEY", "test-admin-key")
	t.Setenv("OPENAI_ADMIN_BASE_URL", server.URL)
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_PROJECT_ID", "")
	t.Setenv("OPENAI_ORG_ID", "")

	summary, err := FetchOpenAISpendSummary(now)
	if err != nil {
		t.Fatalf("FetchOpenAISpendSummary failed: %v", err)
	}

	if summary.Currency != "usd" {
		t.Errorf("expected usd currency, got %s", summary.Currency)
	}

	if summary.Total != 12.0 {
		t.Errorf("expected total 12.0, got %.2f", summary.Total)
	}

	if summary.Last30Days != 3.5 {
		t.Errorf("expected last30 3.5, got %.2f", summary.Last30Days)
	}

	if summary.Today != 0.25 {
		t.Errorf("expected today 0.25, got %.2f", summary.Today)
	}
}

func TestFetchOpenAISpendSummaryRequiresAdminKey(t *testing.T) {
	t.Setenv("OPENAI_ADMIN_KEY", "")

	_, err := FetchOpenAISpendSummary(time.Now())
	if err == nil {
		t.Fatal("expected missing admin key error")
	}

	if err != ErrOpenAIAdminKeyNotSet {
		t.Fatalf("expected ErrOpenAIAdminKeyNotSet, got %v", err)
	}
}
