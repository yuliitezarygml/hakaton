package services

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimitInfo holds the latest rate limit state for one provider.
type RateLimitInfo struct {
	Provider string `json:"provider"`

	// Requests
	LimitRequests     int    `json:"limit_requests"`      // max per window
	RemainingRequests int    `json:"remaining_requests"`  // left in window
	ResetRequests     string `json:"reset_requests"`      // e.g. "6m0s"
	ResetRequestsAt   *int64 `json:"reset_requests_at"`  // unix ms, if parseable

	// Tokens
	LimitTokens     int    `json:"limit_tokens"`      // max per window
	RemainingTokens int    `json:"remaining_tokens"`  // left in window
	ResetTokens     string `json:"reset_tokens"`      // e.g. "1m30s"
	ResetTokensAt   *int64 `json:"reset_tokens_at"`   // unix ms, if parseable

	// Derived
	Throttled   bool   `json:"throttled"`    // true if last response was 429
	StatusCode  int    `json:"status_code"`  // last HTTP status
	UpdatedAt   int64  `json:"updated_at"`   // unix ms
	UpdatedAgo  string `json:"updated_ago"`  // human: "3s ago"
}

var (
	rlMu    sync.RWMutex
	rlStore = map[string]*RateLimitInfo{}
)

// UpdateRateLimit reads rate-limit headers from an HTTP response and stores them.
// provider: "groq" or "openrouter". statusCode: HTTP response status.
func UpdateRateLimit(provider string, resp *http.Response, statusCode int) {
	if resp == nil {
		return
	}

	info := &RateLimitInfo{
		Provider:   provider,
		StatusCode: statusCode,
		Throttled:  statusCode == 429,
		UpdatedAt:  time.Now().UnixMilli(),
	}

	// Requests
	info.LimitRequests     = headerInt(resp, "X-Ratelimit-Limit-Requests")
	info.RemainingRequests = headerInt(resp, "X-Ratelimit-Remaining-Requests")
	info.ResetRequests     = resp.Header.Get("X-Ratelimit-Reset-Requests")

	// Tokens
	info.LimitTokens     = headerInt(resp, "X-Ratelimit-Limit-Tokens")
	info.RemainingTokens = headerInt(resp, "X-Ratelimit-Remaining-Tokens")
	info.ResetTokens     = resp.Header.Get("X-Ratelimit-Reset-Tokens")

	// Parse reset durations to absolute timestamps
	if info.ResetRequests != "" {
		if d, err := time.ParseDuration(info.ResetRequests); err == nil {
			t := time.Now().Add(d).UnixMilli()
			info.ResetRequestsAt = &t
		}
	}
	if info.ResetTokens != "" {
		if d, err := time.ParseDuration(info.ResetTokens); err == nil {
			t := time.Now().Add(d).UnixMilli()
			info.ResetTokensAt = &t
		}
	}

	rlMu.Lock()
	rlStore[provider] = info
	rlMu.Unlock()
}

// GetRateLimits returns a snapshot of all stored rate limit info.
func GetRateLimits() map[string]*RateLimitInfo {
	rlMu.RLock()
	defer rlMu.RUnlock()

	out := map[string]*RateLimitInfo{}
	now := time.Now()
	for k, v := range rlStore {
		cp := *v
		// Human-readable "updated ago"
		ago := now.Sub(time.UnixMilli(v.UpdatedAt))
		switch {
		case ago < time.Minute:
			cp.UpdatedAgo = strconv.Itoa(int(ago.Seconds())) + "s назад"
		default:
			cp.UpdatedAgo = strconv.Itoa(int(ago.Minutes())) + "m назад"
		}
		out[k] = &cp
	}
	return out
}

func headerInt(resp *http.Response, key string) int {
	v := resp.Header.Get(key)
	if v == "" {
		return -1 // -1 = not provided by API
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return -1
	}
	return n
}
