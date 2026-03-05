package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var ErrOpenAIAdminKeyNotSet = errors.New("OPENAI_ADMIN_KEY not set")

var openAICostsMinimumStartTime = time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)

type OpenAISpendSummary struct {
	Currency   string
	Total      float64
	Last30Days float64
	Today      float64
}

type openAICostPage struct {
	Data     []openAICostBucket `json:"data"`
	HasMore  bool               `json:"has_more"`
	NextPage string             `json:"next_page"`
}

type openAICostBucket struct {
	StartTime int64              `json:"start_time"`
	EndTime   int64              `json:"end_time"`
	Results   []openAICostResult `json:"results"`
}

type openAICostResult struct {
	Amount *openAICostAmount `json:"amount"`
}

type openAICostAmount struct {
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

type openAIProject struct {
	CreatedAt int64 `json:"created_at"`
}

var openAIAdminHTTPClient = &http.Client{Timeout: 20 * time.Second}

func FetchOpenAISpendSummary(now time.Time) (*OpenAISpendSummary, error) {
	adminKey := strings.TrimSpace(os.Getenv("OPENAI_ADMIN_KEY"))
	if adminKey == "" {
		return nil, ErrOpenAIAdminKeyNotSet
	}

	now = now.In(time.Local)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	last30Start := now.AddDate(0, 0, -30)

	totalStart := resolveOpenAICostTotalStartTime()

	total, currency, err := fetchOpenAICostRange(totalStart, now)
	if err != nil {
		return nil, err
	}

	last30Days, last30Currency, err := fetchOpenAICostRange(last30Start, now)
	if err != nil {
		return nil, err
	}

	today, todayCurrency, err := fetchOpenAICostRange(dayStart, now)
	if err != nil {
		return nil, err
	}

	summaryCurrency := firstNonEmpty(currency, last30Currency, todayCurrency, "usd")
	return &OpenAISpendSummary{
		Currency:   summaryCurrency,
		Total:      total,
		Last30Days: last30Days,
		Today:      today,
	}, nil
}

func fetchOpenAICostRange(startTime, endTime time.Time) (float64, string, error) {
	adminKey := strings.TrimSpace(os.Getenv("OPENAI_ADMIN_KEY"))
	projectID := strings.TrimSpace(os.Getenv("OPENAI_PROJECT_ID"))

	startTime, endTime = normalizeOpenAICostRange(startTime, endTime)

	startUnix := startTime.Unix()
	endUnix := endTime.Unix()
	if endUnix <= startUnix {
		return 0, "", nil
	}

	client := newOpenAIClient(adminKey, firstNonEmpty(strings.TrimSpace(os.Getenv("OPENAI_ADMIN_BASE_URL")), strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))), openAIAdminHTTPClient)

	total := 0.0
	currency := ""
	nextPage := ""

	for {
		query := url.Values{}
		query.Set("start_time", fmt.Sprintf("%d", startUnix))
		query.Set("end_time", fmt.Sprintf("%d", endUnix))
		query.Set("bucket_width", "1d")
		query.Set("limit", "180")
		if nextPage != "" {
			query.Set("page", nextPage)
		}
		if projectID != "" {
			query.Add("project_ids", projectID)
		}

		var page openAICostPage
		if err := client.Get(context.Background(), "/organization/costs?"+query.Encode(), nil, &page); err != nil {
			return 0, "", fmt.Errorf("failed to fetch OpenAI costs: %w", err)
		}

		for _, bucket := range page.Data {
			for _, result := range bucket.Results {
				if result.Amount == nil {
					continue
				}
				total += result.Amount.Value
				if currency == "" && result.Amount.Currency != "" {
					currency = result.Amount.Currency
				}
			}
		}

		if !page.HasMore || page.NextPage == "" {
			break
		}
		nextPage = page.NextPage
	}

	return total, currency, nil
}

func normalizeOpenAICostRange(startTime, endTime time.Time) (time.Time, time.Time) {
	startUTC := startTime.UTC()
	endUTC := endTime.UTC()

	if sameUTCDate(startUTC, endUTC) {
		nextDayUTC := time.Date(startUTC.Year(), startUTC.Month(), startUTC.Day()+1, 0, 0, 0, 0, time.UTC)
		if nextDayUTC.After(endTime) {
			endTime = nextDayUTC
		}
	}

	return startTime, endTime
}

func sameUTCDate(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func resolveOpenAICostTotalStartTime() time.Time {
	projectID := strings.TrimSpace(os.Getenv("OPENAI_PROJECT_ID"))
	if projectID == "" {
		return openAICostsMinimumStartTime
	}

	adminKey := strings.TrimSpace(os.Getenv("OPENAI_ADMIN_KEY"))
	if adminKey == "" {
		return openAICostsMinimumStartTime
	}

	client := newOpenAIClient(adminKey, firstNonEmpty(strings.TrimSpace(os.Getenv("OPENAI_ADMIN_BASE_URL")), strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))), openAIAdminHTTPClient)

	var project openAIProject
	if err := client.Get(context.Background(), "/organization/projects/"+url.PathEscape(projectID), nil, &project); err != nil {
		return openAICostsMinimumStartTime
	}

	if project.CreatedAt <= 0 {
		return openAICostsMinimumStartTime
	}

	projectCreatedAt := time.Unix(project.CreatedAt, 0).UTC()
	if projectCreatedAt.After(openAICostsMinimumStartTime) {
		return projectCreatedAt
	}

	return openAICostsMinimumStartTime
}

func formatCurrencyAmount(value float64, currency string) string {
	code := strings.ToUpper(strings.TrimSpace(currency))
	if code == "" {
		code = "USD"
	}

	switch code {
	case "USD":
		return fmt.Sprintf("$%.2f", value)
	case "EUR":
		return fmt.Sprintf("EUR %.2f", value)
	default:
		return fmt.Sprintf("%s %.2f", code, value)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
