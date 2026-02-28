# Video Analysis in Telegram Bot — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add video message support to the Telegram bot — upload to Gemini Files API, transcribe speech + describe frames, then pass to existing /api/analyze/stream pipeline.

**Architecture:** User sends video → bot downloads from Telegram → uploads to Gemini Files API → Gemini 2.0 Flash transcribes speech + describes visuals → combined text → existing StreamAnalyze() → result in same format (score 0–10, manipulations, reasons).

**Tech Stack:** Go, Gemini REST API (v1beta) via raw HTTP (no SDK), existing telegram-bot-api/v5

---

## Context

The bot lives in `telegram-bot/`. Key files:
- `main.go` — message handler, `handleMessage()` routes incoming messages
- `analyzer.go` — `StreamAnalyze()`, `AnalysisResult`, `SSEEvent`
- `formatter.go` — `FormatResult()`, `FormatProgress()`, `GetResultKeyboard()`

The Gemini API uses **three steps** for video:
1. Upload file → get `file_uri` and `file_name`
2. Poll until file state = `"ACTIVE"` (usually 1–3 sec for short videos)
3. Send `generateContent` request referencing the file

Free tier: 1500 req/day, 50 req/min at `aistudio.google.com`.

---

## Task 1: Add GEMINI_API_KEY to environment

**Files:**
- Modify: `D:/project/openrouter-web/.env`
- Modify: `D:/project/openrouter-web/.env.example`

**Step 1: Add to .env**

Open `.env` (root of project) and add:

```
GEMINI_API_KEY=your_key_here
```

Get a free key at: https://aistudio.google.com → "Get API key"

**Step 2: Add to .env.example**

```
GEMINI_API_KEY=                # Google AI Studio key for video analysis (free: 1500 req/day)
```

**Step 3: Verify the bot reads it**

The bot already calls `godotenv.Load("../.env")` in `main.go:36`, so the key will be available via `os.Getenv("GEMINI_API_KEY")`. No code change needed for loading.

**Step 4: Commit**

```bash
cd D:/project/openrouter-web
git add .env.example
git commit -m "feat(bot): add GEMINI_API_KEY to env example"
```

---

## Task 2: Create gemini.go — Gemini Files API client

**Files:**
- Create: `telegram-bot/gemini.go`

This file contains all Gemini logic: upload video bytes, poll for ACTIVE state, then call generateContent to get transcript + visual description.

**Step 1: Create the file**

Create `telegram-bot/gemini.go` with this exact content:

```go
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
	Name        string `json:"name"`        // e.g. "files/abc123"
	DisplayName string `json:"displayName"`
	MimeType    string `json:"mimeType"`
	State       string `json:"state"` // PROCESSING, ACTIVE, FAILED
	URI         string `json:"uri"`   // used in generateContent
}

// UploadVideoToGemini uploads raw video bytes to the Gemini Files API.
// Returns the file URI and name, or an error.
func UploadVideoToGemini(ctx context.Context, apiKey string, data []byte, mimeType string) (*GeminiFile, error) {
	// Build multipart body:
	// Part 1: JSON metadata
	// Part 2: binary video data
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Metadata part
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

	// Video data part
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

	url := geminiBase + "/upload/v1beta/files?key=" + apiKey
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

	body, _ := io.ReadAll(resp.Body)
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

// WaitForGeminiFile polls the file status until it's ACTIVE or times out.
func WaitForGeminiFile(ctx context.Context, apiKey, fileName string) error {
	url := geminiBase + "/" + fileName + "?key=" + apiKey
	deadline := time.Now().Add(30 * time.Second)

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
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var f GeminiFile
		if err := json.Unmarshal(body, &f); err != nil {
			return fmt.Errorf("parse poll response: %w", err)
		}

		switch f.State {
		case "ACTIVE":
			return nil
		case "FAILED":
			return fmt.Errorf("Gemini file processing failed")
		}

		// Still PROCESSING — wait and retry
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for Gemini file to become active")
}

// AnalyzeVideoWithGemini sends the uploaded video to Gemini Flash for transcription
// and visual description. Returns the combined text.
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

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini generateContent HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response: candidates[0].content.parts[0].text
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

// DeleteGeminiFile cleans up the uploaded file from Gemini Files API.
// Errors are silently ignored (best-effort cleanup).
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
```

**Step 2: Build to check for compile errors**

```bash
cd D:/project/openrouter-web/telegram-bot
go build ./...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add telegram-bot/gemini.go
git commit -m "feat(bot): add Gemini Files API client for video upload and analysis"
```

---

## Task 3: Add video handler to main.go

**Files:**
- Modify: `telegram-bot/main.go`

**Step 1: Add `handleVideo` function**

Add this function BEFORE the `handleCallback` function (around line 447):

```go
// ── Video handler ─────────────────────────────────────────────────

func handleVideo(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		send(chatID, "❌ Видеоанализ недоступен: GEMINI_API_KEY не настроен.")
		return
	}

	// Get file info from Telegram
	var fileID string
	var fileSize int
	var mimeType string

	switch {
	case msg.Video != nil:
		fileID = msg.Video.FileID
		fileSize = msg.Video.FileSize
		mimeType = msg.Video.MimeType
		if mimeType == "" {
			mimeType = "video/mp4"
		}
	case msg.Animation != nil:
		fileID = msg.Animation.FileID
		fileSize = msg.Animation.FileSize
		mimeType = "video/mp4"
	default:
		return
	}

	// 50 MB limit (Telegram Bot API limit)
	const maxBytes = 50 * 1024 * 1024
	if fileSize > maxBytes {
		send(chatID, fmt.Sprintf("❌ Видео слишком большое (%d МБ). Максимум — 50 МБ.", fileSize/1024/1024))
		return
	}

	initMsg := sendAndGet(chatID, "🎬 <b>Видео получено</b>\n\n<code>Загружаю в Gemini для анализа...</code>")
	if initMsg == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	registerAnalysis(chatID, cancel)

	go func() {
		defer func() {
			cancel()
			unregisterAnalysis(chatID)
		}()
		runVideoAnalysis(ctx, chatID, initMsg.MessageID, fileID, mimeType)
	}()
}

func runVideoAnalysis(ctx context.Context, chatID int64, msgID int, fileID, mimeType string) {
	geminiKey := os.Getenv("GEMINI_API_KEY")

	// Step 1: Get Telegram download URL and download the file
	edit(chatID, msgID, "🎬 <b>Видео получено</b>\n\n<code>Скачиваю файл...</code>")

	fileURL, err := bot.GetFileDirectURL(fileID)
	if err != nil {
		edit(chatID, msgID, "❌ <b>Ошибка:</b> не удалось получить ссылку на файл.\n<code>"+escHTML(err.Error())+"</code>")
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		edit(chatID, msgID, "❌ <b>Ошибка:</b> не удалось создать запрос.\n<code>"+escHTML(err.Error())+"</code>")
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		edit(chatID, msgID, "❌ <b>Ошибка:</b> не удалось скачать видео.\n<code>"+escHTML(err.Error())+"</code>")
		return
	}
	defer resp.Body.Close()
	videoBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		edit(chatID, msgID, "❌ <b>Ошибка:</b> не удалось прочитать файл.\n<code>"+escHTML(err.Error())+"</code>")
		return
	}

	// Step 2: Upload to Gemini Files API
	edit(chatID, msgID, "🎬 <b>Видео получено</b>\n\n<code>Загружаю в Gemini...</code>")
	geminiFile, err := UploadVideoToGemini(ctx, geminiKey, videoBytes, mimeType)
	if err != nil {
		edit(chatID, msgID, "❌ <b>Ошибка загрузки в Gemini:</b>\n<code>"+escHTML(err.Error())+"</code>")
		return
	}
	defer DeleteGeminiFile(geminiKey, geminiFile.Name)

	// Step 3: Wait for file to become ACTIVE
	edit(chatID, msgID, "🎬 <b>Видео получено</b>\n\n<code>Gemini обрабатывает файл...</code>")
	if err := WaitForGeminiFile(ctx, geminiKey, geminiFile.Name); err != nil {
		edit(chatID, msgID, "❌ <b>Ошибка обработки файла:</b>\n<code>"+escHTML(err.Error())+"</code>")
		return
	}

	// Step 4: Transcribe speech + describe visuals
	edit(chatID, msgID, "🎬 <b>Видео получено</b>\n\n<code>Gemini расшифровывает речь и кадры...</code>")
	description, err := AnalyzeVideoWithGemini(ctx, geminiKey, geminiFile.URI, mimeType)
	if err != nil {
		edit(chatID, msgID, "❌ <b>Ошибка анализа Gemini:</b>\n<code>"+escHTML(err.Error())+"</code>")
		return
	}

	if len([]rune(description)) < 30 {
		edit(chatID, msgID, "⚠️ Gemini не смог извлечь достаточно информации из видео.")
		return
	}

	// Show what Gemini extracted before running disinformation analysis
	preview := description
	if len([]rune(preview)) > 300 {
		runes := []rune(preview)
		preview = string(runes[:300]) + "..."
	}
	edit(chatID, msgID, fmt.Sprintf(
		"🎬 <b>Видео расшифровано</b>\n\n<code>%s</code>\n\n⏳ <b>Анализирую на дезинформацию...</b>",
		escHTML(preview),
	))

	// Step 5: Run through existing disinformation analysis pipeline
	payload := map[string]any{"text": description}
	runAnalysis(ctx, chatID, msgID, payload, "")
}
```

**Step 2: Add video routing in `handleMessage`**

Find this block in `handleMessage` (around line 152–185):

```go
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
```

Replace with:

```go
	// Video / animation handling
	if msg.Video != nil || msg.Animation != nil {
		handleVideo(msg)
		return
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
```

**Step 3: Add `io` import**

Find the import block in `main.go`:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
```

`io` is already imported (line 8) — no change needed.

**Step 4: Update /start and /help text to mention video**

Find `startText()` in `main.go`. Replace with:

```go
func startText() string {
	return `🔍 <b>Text Analyzer Bot</b>

Я анализирую статьи, тексты и <b>видео</b> на предмет дезинформации, манипуляций и логических ошибок.

<b>Как использовать:</b>
• Отправьте <b>URL</b> статьи — и я её проанализирую
• Вставьте <b>текст</b> (мин. 100 символов) напрямую
• Отправьте <b>видео</b> (~10 сек) — расшифрую речь + опишу кадры
• <b>Перешлите</b> любое сообщение из канала или чата

<b>Команды:</b>
/cancel — остановить текущий анализ
/help — помощь`
}
```

Find `helpText()`. Add video section:

```go
func helpText() string {
	return `📖 <b>Помощь</b>

<b>Отправить URL:</b>
<code>https://example.com/article</code>

<b>Отправить текст:</b>
Просто вставьте текст статьи (минимум 100 символов).

🎬 <b>Отправить видео (~10 сек):</b>
Бот расшифрует речь через Gemini AI и опишет содержимое кадров,
затем проверит на дезинформацию. Максимум 50 МБ.

🔁 <b>Переслать сообщение из канала:</b>
Перешлите любое сообщение — бот автоматически обнаружит источник.

<b>Результат включает:</b>
• Балл достоверности (0–10)
• Вердикт (достоверно / сомнительно / дезинформация)
• Краткое резюме
• Список манипуляций и почему они таковы
• Логические ошибки
• Утверждения без доказательств

<b>Команды:</b>
/cancel — остановить анализ
/start — главное меню`
}
```

**Step 5: Build**

```bash
cd D:/project/openrouter-web/telegram-bot
go build ./...
```

Expected: no errors.

**Step 6: Commit**

```bash
git add telegram-bot/main.go
git commit -m "feat(bot): add video analysis via Gemini Files API"
```

---

## Task 4: Update Dockerfile (if deployed in Docker)

**Files:**
- Modify: `telegram-bot/Dockerfile`

**Step 1: Read existing Dockerfile**

```bash
cat D:/project/openrouter-web/telegram-bot/Dockerfile
```

**Step 2: Ensure GEMINI_API_KEY is passed through**

In `docker-compose.yml` (project root), add to the telegram-bot service's `env_file` section — it should already be there if `.env` is loaded. Verify the bot service has:

```yaml
env_file:
  - .env
```

No Dockerfile change needed — environment variables come from the host via `env_file`.

**Step 3: Commit if changed**

```bash
git add docker-compose.yml
git commit -m "chore: ensure GEMINI_API_KEY passed to bot container"
```

---

## Task 5: Manual smoke test

**Step 1: Start the backend**

```bash
cd D:/project/openrouter-web
go run main.go
```

Verify: `Listening on :8080`

**Step 2: Start the bot**

```bash
cd D:/project/openrouter-web/telegram-bot
go run .
```

Verify: `[bot] Запущен как @YourBotName | API: ...`

**Step 3: Test video in Telegram**

Send a short video (~5-10 sec) with someone speaking to the bot.

Expected sequence of bot messages (edits):
1. `🎬 Видео получено — Скачиваю файл...`
2. `🎬 Видео получено — Загружаю в Gemini...`
3. `🎬 Видео получено — Gemini обрабатывает файл...`
4. `🎬 Видео получено — Gemini расшифровывает речь и кадры...`
5. `🎬 Видео расшифровано — [preview of transcript] — ⏳ Анализирую...`
6. Final result with score 0–10, manipulations, etc.

**Step 4: Test error cases**

- Send a video without GEMINI_API_KEY set → expect `❌ Видеоанализ недоступен`
- Send a silent video (no speech) → expect visual-only description + analysis
- Use /cancel during analysis → expect analysis to stop

**Step 5: Final commit**

```bash
git add -A
git commit -m "feat(bot): video disinformation analysis via Gemini Flash — complete"
```

---

## Notes

- **Gemini file cleanup:** `DeleteGeminiFile` runs as `defer` so the uploaded file is cleaned up even on errors.
- **Context propagation:** All Gemini calls use the same context as the analysis — if user cancels with `/cancel`, the upload/analysis stops.
- **Video MIME types:** Telegram sends `video/mp4` for most videos. GIFs/animations come as `msg.Animation` with `video/mp4`. Both are handled.
- **No ffmpeg required:** Gemini handles extraction natively.
- **Rate limits:** 50 req/min free tier. For a small bot this is fine. If needed, add a semaphore.
