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

const geminiBase = "https://generativelanguage.googleapis.com/v1beta"

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

		time.Sleep(2 * time.Second)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return fmt.Errorf("timeout waiting for Gemini file to become active")
}

// AnalyzeVideoWithGemini sends the uploaded video to Gemini Flash for transcription
// and visual description. Returns combined text.
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

	url := geminiBase + "/models/gemini-2.0-flash:generateContent?key=" + apiKey
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("generateContent request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini generateContent HTTP %d: %s", resp.StatusCode, string(respBody))
	}

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

// DeleteGeminiFile cleans up the uploaded file. Errors silently ignored (best-effort).
func DeleteGeminiFile(apiKey, fileName string) {
	url := geminiBase + "/" + fileName + "?key=" + apiKey
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
