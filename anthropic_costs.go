package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var ErrAnthropicAdminKeyNotSet = errors.New("ANTHROPIC_ADMIN_KEY not set")

var anthropicCostsMinimumStartTime = time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)

type AnthropicSpendSummary struct {
	Currency   string
	Total      float64
	Last30Days float64
	Today      float64
}

type anthropicCostPage struct {
	Data     []any  `json:"data"`
	HasMore  bool   `json:"has_more"`
	NextPage string `json:"next_page"`
}

var anthropicAdminHTTPClient = &http.Client{Timeout: 20 * time.Second}

func FetchAnthropicSpendSummary(now time.Time) (*AnthropicSpendSummary, error) {
	if anthropicAdminKey() == "" {
		return nil, ErrAnthropicAdminKeyNotSet
	}

	now = now.In(time.Local)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	last30Start := now.AddDate(0, 0, -30)

	total, currency, err := fetchAnthropicCostRange(anthropicCostsMinimumStartTime, now)
	if err != nil {
		return nil, err
	}

	last30Days, last30Currency, err := fetchAnthropicCostRange(last30Start, now)
	if err != nil {
		return nil, err
	}

	today, todayCurrency, err := fetchAnthropicCostRange(dayStart, now)
	if err != nil {
		return nil, err
	}

	return &AnthropicSpendSummary{
		Currency:   firstNonEmpty(currency, last30Currency, todayCurrency, "usd"),
		Total:      total,
		Last30Days: last30Days,
		Today:      today,
	}, nil
}

func fetchAnthropicCostRange(startTime, endTime time.Time) (float64, string, error) {
	adminKey := anthropicAdminKey()
	if adminKey == "" {
		return 0, "", ErrAnthropicAdminKeyNotSet
	}

	baseURL := strings.TrimRight(firstNonEmpty(strings.TrimSpace(os.Getenv("ANTHROPIC_ADMIN_BASE_URL")), strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")), "https://api.anthropic.com"), "/")
	total := 0.0
	currency := ""
	nextPage := ""

	for {
		query := url.Values{}
		query.Set("starting_at", startTime.UTC().Format(time.RFC3339))
		query.Set("ending_at", endTime.UTC().Format(time.RFC3339))
		query.Set("bucket_width", "1d")
		query.Set("limit", "31")
		if nextPage != "" {
			query.Set("page", nextPage)
		}

		req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/organizations/cost_report?"+query.Encode(), nil)
		if err != nil {
			return 0, "", fmt.Errorf("failed to create Anthropic cost request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("x-api-key", adminKey)

		resp, err := anthropicAdminHTTPClient.Do(req)
		if err != nil {
			return 0, "", fmt.Errorf("failed to fetch Anthropic costs: %w", err)
		}

		var page anthropicCostPage
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			return 0, "", fmt.Errorf("Anthropic cost API error (status %d)", resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			_ = resp.Body.Close()
			return 0, "", fmt.Errorf("failed to decode Anthropic cost response: %w", err)
		}
		_ = resp.Body.Close()

		pageTotal, pageCurrency := sumAnthropicCostFields(page.Data)
		total += pageTotal
		currency = firstNonEmpty(currency, pageCurrency)

		if !page.HasMore || page.NextPage == "" {
			break
		}
		nextPage = page.NextPage
	}

	return total, firstNonEmpty(currency, "usd"), nil
}

func anthropicAdminKey() string {
	return firstNonEmpty(strings.TrimSpace(os.Getenv("ANTHROPIC_ADMIN_KEY")), strings.TrimSpace(os.Getenv("ANTHROPIC_ADMIN_API_KEY")))
}

func sumAnthropicCostFields(node any) (float64, string) {
	switch value := node.(type) {
	case map[string]any:
		total := 0.0
		currency := ""

		if amount, ok := extractAnthropicAmount(value["amount"]); ok {
			total += amount.value
			currency = firstNonEmpty(currency, amount.currency)
		}
		if amount, ok := extractAnthropicAmount(value["cost"]); ok {
			total += amount.value
			currency = firstNonEmpty(currency, amount.currency)
		}
		if amount, ok := extractAnthropicAmount(value["cost_usd"]); ok {
			total += amount.value
			currency = firstNonEmpty(currency, amount.currency)
		}

		for _, child := range value {
			childTotal, childCurrency := sumAnthropicCostFields(child)
			total += childTotal
			currency = firstNonEmpty(currency, childCurrency)
		}

		return total, currency
	case []any:
		total := 0.0
		currency := ""
		for _, child := range value {
			childTotal, childCurrency := sumAnthropicCostFields(child)
			total += childTotal
			currency = firstNonEmpty(currency, childCurrency)
		}
		return total, currency
	default:
		return 0, ""
	}
}

type anthropicAmount struct {
	value    float64
	currency string
}

func extractAnthropicAmount(node any) (anthropicAmount, bool) {
	switch value := node.(type) {
	case float64:
		return anthropicAmount{value: value, currency: "usd"}, true
	case string:
		if amount, ok := monetaryValue(value); ok {
			return anthropicAmount{value: amount, currency: "usd"}, true
		}
	case map[string]any:
		if usd, ok := monetaryValue(value["usd"]); ok {
			return anthropicAmount{value: usd, currency: "usd"}, true
		}
		if rawValue, ok := monetaryValue(value["value"]); ok {
			currency := "usd"
			if rawCurrency, ok := value["currency"].(string); ok {
				currency = strings.TrimSpace(strings.ToLower(rawCurrency))
			}
			return anthropicAmount{value: rawValue, currency: currency}, true
		}
	}

	return anthropicAmount{}, false
}

func monetaryValue(node any) (float64, bool) {
	switch value := node.(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0, false
		}
		return parsed / 100, true
	}
	return 0, false
}
