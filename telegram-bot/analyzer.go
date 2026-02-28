package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type SSEEvent struct {
	Type string
	Data string
}

type AnalysisResult struct {
	CredibilityScore int      `json:"credibility_score"`
	Summary          string   `json:"summary"`
	Manipulations    []string `json:"manipulations"`
	LogicalIssues    []string `json:"logical_issues"`
	FactCheck        *struct {
		MissingEvidence []string `json:"missing_evidence"`
		OpinionsAsFacts []string `json:"opinions_as_facts"`
		FoundEvidence   []string `json:"found_evidence"`
	} `json:"fact_check"`
}

// StreamAnalyze sends a request to /api/analyze/stream and calls cb for each SSE event.
func StreamAnalyze(ctx context.Context, apiBase string, payload map[string]any, cb func(SSEEvent)) error {
	// Обрезаем текст до безопасного размера (~3500 символов) чтобы избежать ошибки 413
	const maxRunes = 3500
	if text, ok := payload["text"].(string); ok {
		runes := []rune(text)
		if len(runes) > maxRunes {
			payload["text"] = string(runes[:maxRunes]) + "\n\n[...текст обрезан до 3500 символов для анализа...]"
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiBase+"/api/analyze/stream", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API вернул статус %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	var eventType, eventData string
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(line[6:])
		case strings.HasPrefix(line, "data:"):
			eventData = strings.TrimSpace(line[5:])
		case line == "" && eventType != "":
			cb(SSEEvent{Type: eventType, Data: eventData})
			eventType = ""
			eventData = ""
		}
	}
	return scanner.Err()
}

// ParseResult parses the JSON result from the "result" SSE event.
func ParseResult(data string) (*AnalysisResult, error) {
	var r AnalysisResult
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return nil, err
	}
	return &r, nil
}
