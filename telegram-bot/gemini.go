package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const (
	geminiBase   = "https://generativelanguage.googleapis.com/v1beta" // Files API
	geminiV1Base = "https://generativelanguage.googleapis.com/v1"     // generateContent (stable models)
)

// GeminiFile represents the uploaded file metadata from Gemini Files API.
type GeminiFile struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	MimeType    string `json:"mimeType"`
	State       string `json:"state"` // PROCESSING, ACTIVE, FAILED
	URI         string `json:"uri"`
}

// UploadVideoToGemini uploads raw video bytes to the Gemini Files API.
// Returns the file metadata, or an error.
func UploadVideoToGemini(ctx context.Context, apiKey string, data []byte, mimeType string) (*GeminiFile, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Part 1: JSON metadata
	metaPart, err := mw.CreatePart(map[string][]string{
		"Content-Type": {"application/json"},
	})
	if err != nil {
		return nil, fmt.Errorf("create meta part: %w", err)
	}
	meta := map[string]any{"file": map[string]string{"display_name": "video"}}
	if err := json.NewEncoder(metaPart).Encode(meta); err != nil {
		return nil, fmt.Errorf("encode meta: %w", err)
	}

	// Part 2: binary video data
	dataPart, err := mw.CreatePart(map[string][]string{
		"Content-Type": {mimeType},
	})
	if err != nil {
		return nil, fmt.Errorf("create data part: %w", err)
	}
	if _, err := dataPart.Write(data); err != nil {
		return nil, fmt.Errorf("write data: %w", err)
	}
	mw.Close()

	url := "https://generativelanguage.googleapis.com/upload/v1beta/files?key=" + apiKey
	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "multipart/related; boundary="+mw.Boundary())
	req.Header.Set("X-Goog-Upload-Protocol", "multipart")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini upload HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		File GeminiFile `json:"file"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse upload response: %w", err)
	}
	return &result.File, nil
}

// WaitForGeminiFile polls the file status until ACTIVE or timeout (30s).
func WaitForGeminiFile(ctx context.Context, apiKey, fileName string) error {
	url := geminiBase + "/" + fileName + "?key=" + apiKey
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("poll request: %w", err)
		}
		readBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read poll response body: %w", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Gemini poll HTTP %d: %s", resp.StatusCode, string(readBody))
		}

		var f GeminiFile
		if err := json.Unmarshal(readBody, &f); err != nil {
			return fmt.Errorf("parse poll response: %w", err)
		}

		switch f.State {
		case "ACTIVE":
			return nil
		case "FAILED":
			return fmt.Errorf("Gemini file processing failed")
		}

		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("timeout waiting for Gemini file to become active")
}

// geminiModels is the ordered list of (model, apiBase) pairs to try on quota errors.
var geminiModels = []struct{ name, base string }{
	{"gemini-1.5-flash", geminiV1Base},
	{"gemini-1.5-flash-8b", geminiV1Base},
	{"gemini-2.0-flash-lite", geminiBase},
}

// AnalyzeVideoWithGemini sends the uploaded video to Gemini Flash for transcription
// and visual description. Tries models in order, retrying once on 429.
func AnalyzeVideoWithGemini(ctx context.Context, apiKey, fileURI, mimeType string) (string, error) {
	prompt := `Analyze this video carefully and return exactly two sections:

SPEECH TRANSCRIPT:
Transcribe all spoken words verbatim. If there is no speech, write "No speech detected."

VISUAL DESCRIPTION:
Describe what is visually shown: setting, people present, text on screen, graphics, maps, charts, emotional tone, any notable visual elements.`

	reqBody := map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{
						"file_data": map[string]string{
							"mime_type": mimeType,
							"file_uri":  fileURI,
						},
					},
					map[string]any{
						"text": prompt,
					},
				},
			},
		},
		"generationConfig": map[string]any{
			"maxOutputTokens": 1024,
			"temperature":     0.1,
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for _, m := range geminiModels {
		result, err := doGenerateContent(ctx, apiKey, m.base, m.name, bodyBytes)
		if err == nil {
			return result, nil
		}
		lastErr = err
		// Only fall through to next model on quota/rate-limit/not-found errors
		if !isQuotaError(err) {
			return "", err
		}
	}
	return "", lastErr
}

// doGenerateContent calls generateContent for one model. On 429 waits up to 90s then retries once.
func doGenerateContent(ctx context.Context, apiKey, apiBase, model string, bodyBytes []byte) (string, error) {
	url := apiBase + "/models/" + model + ":generateContent?key=" + apiKey

	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return "", fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("generateContent request: %w", err)
		}
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("read response body: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			var result struct {
				Candidates []struct {
					Content struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"content"`
				} `json:"candidates"`
			}
			if err := json.Unmarshal(respBody, &result); err != nil {
				return "", fmt.Errorf("parse response: %w", err)
			}
			if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
				return "", fmt.Errorf("empty response from Gemini")
			}
			return strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text), nil
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			// Parse retryDelay from response, cap at 90s
			delay := parseRetryDelay(respBody)
			if delay > 90*time.Second {
				return "", fmt.Errorf("Gemini quota exceeded for model %s (retry in %s)", model, delay.Round(time.Second))
			}
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			continue
		}

		return "", fmt.Errorf("Gemini generateContent HTTP %d (model %s): %s", resp.StatusCode, model, string(respBody))
	}
	return "", fmt.Errorf("Gemini quota exceeded for model %s", model)
}

// parseRetryDelay extracts the retryDelay duration from a Gemini 429 response body.
// Falls back to 60s if parsing fails.
func parseRetryDelay(body []byte) time.Duration {
	var errResp struct {
		Error struct {
			Details []struct {
				RetryDelay string `json:"retryDelay"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil {
		for _, d := range errResp.Error.Details {
			if d.RetryDelay != "" {
				// Format: "56.131542619s" or "56s"
				if dur, err := time.ParseDuration(d.RetryDelay); err == nil {
					return dur
				}
				// Try stripping fractional seconds: "56.13s" → try as-is first, then truncate
				if dur, err := time.ParseDuration(strings.Split(d.RetryDelay, ".")[0] + "s"); err == nil {
					return dur
				}
			}
		}
	}
	return 60 * time.Second
}

// isQuotaError returns true if err indicates a quota, rate-limit, or model-not-found problem
// — all cases where trying the next model makes sense.
func isQuotaError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 429") ||
		strings.Contains(s, "HTTP 404") ||
		strings.Contains(s, "quota exceeded")
}

// DeleteGeminiFile cleans up the uploaded file. Errors silently ignored (best-effort).
func DeleteGeminiFile(apiKey, fileName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	url := geminiBase + "/" + fileName + "?key=" + apiKey
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
