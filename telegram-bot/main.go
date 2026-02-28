package main

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

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

var (
	apiBase string
	bot     *tgbotapi.BotAPI

	// Track active analyses per chat (chatID → cancel func)
	activeMu sync.Mutex
	active   = map[int64]context.CancelFunc{}

	// For re-scan (msgID -> payload)
	historyMu sync.Mutex
	history   = map[string]map[string]any{}
)

func main() {
	// In Docker env vars are injected via env_file — godotenv is a no-op.
	// Locally: try root project .env first, then local .env.
	if os.Getenv("TELEGRAM_TOKEN") == "" {
		if err := godotenv.Load("../.env"); err != nil {
			_ = godotenv.Load()
		}
	}

	token := os.Getenv("TELEGRAM_TOKEN")
	if token == "" {
		log.Fatal("[bot] TELEGRAM_TOKEN не задан в .env")
	}

	apiBase = os.Getenv("API_BASE")
	if apiBase == "" {
		apiBase = "https://apich.sinkdev.dev"
	}

	var err error
	bot, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("[bot] Ошибка инициализации: %v", err)
	}

	log.Printf("[bot] Запущен как @%s | API: %s", bot.Self.UserName, apiBase)

	if webhookURL := os.Getenv("WEBHOOK_URL"); webhookURL != "" {
		runWebhook(webhookURL)
	} else {
		runPolling()
	}
}

// ── Polling mode (dev / no public URL) ───────────────────────────

func runPolling() {
	// Remove any previously registered webhook
	if _, err := bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: false}); err != nil {
		log.Printf("[bot] DeleteWebhook: %v", err)
	}

	log.Println("[bot] Режим: POLLING")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			go handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			go handleCallback(update.CallbackQuery)
		}
	}
}

// ── Webhook mode (production) ─────────────────────────────────────

func runWebhook(baseURL string) {
	port := os.Getenv("WEBHOOK_PORT")
	if port == "" {
		port = "8443"
	}

	// Path contains bot token — acts as a secret, no extra auth needed
	path := "/" + bot.Token
	fullURL := strings.TrimRight(baseURL, "/") + path

	wh, err := tgbotapi.NewWebhook(fullURL)
	if err != nil {
		log.Fatalf("[bot] NewWebhook: %v", err)
	}

	if _, err := bot.Request(wh); err != nil {
		log.Fatalf("[bot] Ошибка установки webhook: %v", err)
	}

	info, err := bot.GetWebhookInfo()
	if err != nil {
		log.Fatalf("[bot] GetWebhookInfo: %v", err)
	}
	if info.LastErrorDate != 0 {
		log.Printf("[bot] ⚠ Последняя ошибка webhook: %s", info.LastErrorMessage)
	}

	log.Printf("[bot] Режим: WEBHOOK")
	log.Printf("[bot] URL:  %s", fullURL)
	log.Printf("[bot] Порт: :%s", port)

	updates := bot.ListenForWebhook(path)

	go func() {
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatalf("[bot] HTTP сервер упал: %v", err)
		}
	}()

	log.Printf("[bot] Webhook сервер слушает :%s", port)

	for update := range updates {
		if update.Message != nil {
			go handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			go handleCallback(update.CallbackQuery)
		}
	}
}

// ── Message handler ──────────────────────────────────────────────

func handleMessage(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	// ── Forwarded message detection ──
	if msg.ForwardFromChat != nil || msg.ForwardFrom != nil {
		handleForwarded(msg)
		return
	}

	// Video / animation handling
	if msg.Video != nil || msg.Animation != nil {
		handleVideo(msg)
		return
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	switch {
	case text == "/start":
		send(chatID, startText())
		return

	case text == "/help":
		send(chatID, helpText())
		return

	case text == "/cancel":
		cancelAnalysis(chatID)
		send(chatID, "⛔ Анализ отменён.")
		return

	case text == "/history":
		send(chatID, "📋 История сохраняется на стороне сервера. Используйте <b>Admin Panel</b> для просмотра.")
		return
	}

	// Determine payload
	var payload map[string]any
	if isURL(text) {
		payload = map[string]any{"url": text}
	} else if len([]rune(text)) >= 100 {
		payload = map[string]any{"text": text}
	} else {
		send(chatID, "❓ Отправьте <b>URL</b> статьи или <b>текст</b> для анализа (минимум 100 символов).\n\nПример:\n<code>https://example.com/article</code>")
		return
	}

	startAnalysisForChat(chatID, payload, "")
}

// ── Forwarded message handler ─────────────────────────────────────

func handleForwarded(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	// Extract source name
	sourceName := ""
	sourceLink := ""

	switch {
	case msg.ForwardFromChat != nil:
		// Forwarded from channel or group
		chat := msg.ForwardFromChat
		if chat.Title != "" {
			sourceName = chat.Title
		}
		if chat.UserName != "" {
			sourceLink = "https://t.me/" + chat.UserName
		}
	case msg.ForwardFrom != nil:
		// Forwarded from user
		u := msg.ForwardFrom
		if u.UserName != "" {
			sourceName = "@" + u.UserName
			sourceLink = "https://t.me/" + u.UserName
		} else {
			sourceName = strings.TrimSpace(u.FirstName + " " + u.LastName)
		}
	default:
		// Privacy-hidden sender — try SenderUserName field (may be empty)
		if msg.ForwardSenderName != "" {
			sourceName = msg.ForwardSenderName
		}
	}

	// Get text from message or caption
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		text = strings.TrimSpace(msg.Caption)
	}

	// If there's a URL in the entities — prefer it
	var detectedURL string
	for _, e := range append(msg.Entities, msg.CaptionEntities...) {
		if e.Type == "url" || e.Type == "text_link" {
			if e.URL != "" {
				detectedURL = e.URL
			} else if len(text) >= e.Offset+e.Length {
				runes := []rune(text)
				if e.Offset+e.Length <= len(runes) {
					detectedURL = string(runes[e.Offset : e.Offset+e.Length])
				}
			}
			break
		}
	}

	// Build payload
	var payload map[string]any
	if detectedURL != "" && isURL(detectedURL) {
		payload = map[string]any{"url": detectedURL}
	} else if isURL(text) {
		payload = map[string]any{"url": text}
	} else if len([]rune(text)) >= 30 {
		payload = map[string]any{"text": text}
	} else {
		// Not enough content
		msg2 := "🔄 <b>Пересланное сообщение получено</b>"
		if sourceName != "" {
			if sourceLink != "" {
				msg2 += fmt.Sprintf("\n📢 Источник: <a href=\"%s\">%s</a>", sourceLink, escHTML(sourceName))
			} else {
				msg2 += fmt.Sprintf("\n📢 Источник: <b>%s</b>", escHTML(sourceName))
			}
		}
		msg2 += "\n\n❌ Сообщение слишком короткое для анализа (минимум 30 символов).\nДобавьте сообщение текстом или перешлите URL статьи."
		send(chatID, msg2)
		return
	}

	// Source label to show in result
	sourceLabel := ""
	if sourceName != "" {
		if sourceLink != "" {
			sourceLabel = fmt.Sprintf("<a href=\"%s\">%s</a>", sourceLink, escHTML(sourceName))
		} else {
			sourceLabel = escHTML(sourceName)
		}
	}

	startAnalysisForChat(chatID, payload, sourceLabel)
}

// ── Shared analysis starter ───────────────────────────────────────

func startAnalysisForChat(chatID int64, payload map[string]any, sourceLabel string) {
	cancelAnalysis(chatID)

	// Build init message showing source if forwarded
	initText := "⏳ <b>Анализирую...</b>\n\n<code>Загружаю страницу...</code>"
	if sourceLabel != "" {
		initText = fmt.Sprintf("⏳ <b>Анализирую...</b>\n📢 Источник: %s\n\n<code>Загружаю страницу...</code>", sourceLabel)
	}

	initMsg := sendAndGet(chatID, initText)
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
		runAnalysis(ctx, chatID, initMsg.MessageID, payload, sourceLabel)
	}()
}

// ── Analysis runner ──────────────────────────────────────────────

func runAnalysis(ctx context.Context, chatID int64, msgID int, payload map[string]any, sourceLabel string) {
	var (
		progressLines []string
		lastEdit      time.Time
		finalResult   *AnalysisResult
		analysisErr   string
	)

	// Build re-scan data
	var reScanData string
	payloadJSON, _ := json.Marshal(payload)
	if len(payloadJSON) < 60 {
		reScanData = string(payloadJSON)
	} else {
		// Use memory key
		key := fmt.Sprintf("%d:%d", chatID, msgID)
		historyMu.Lock()
		history[key] = payload
		historyMu.Unlock()
		reScanData = "key:" + key
	}

	err := StreamAnalyze(ctx, apiBase, payload, func(ev SSEEvent) {
		switch ev.Type {
		case "start", "progress":
			progressLines = append(progressLines, ev.Data)
			// Throttle edits: max 1 per 2s
			if time.Since(lastEdit) >= 2*time.Second {
				edit(chatID, msgID, FormatProgress(progressLines))
				lastEdit = time.Now()
			}

		case "result":
			r, parseErr := ParseResult(ev.Data)
			if parseErr == nil {
				finalResult = r
			}

		case "error":
			analysisErr = ev.Data
		}
	})

	switch {
	case ctx.Err() == context.Canceled:
		// User cancelled — message already updated in /cancel handler
		return

	case finalResult != nil:
		shareURL := ""
		if _, ok := payload["url"].(string); ok {
			shareURL = requestShareURL(finalResult)
		} else if _, ok := payload["text"].(string); ok {
			shareURL = requestShareURL(finalResult)
		}

		editWithKeyboard(chatID, msgID, FormatResult(finalResult, sourceLabel), GetResultKeyboard(shareURL, reScanData))

	case analysisErr != "":
		edit(chatID, msgID, "❌ <b>Ошибка анализа:</b>\n<code>"+escHTML(analysisErr)+"</code>")

	case err != nil:
		edit(chatID, msgID, "❌ <b>Ошибка связи с API:</b>\n<code>"+escHTML(err.Error())+"</code>")

	default:
		edit(chatID, msgID, "⚠️ Анализ завершён без результата.")
	}
}

// ── Telegram helpers ─────────────────────────────────────────────

func send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	bot.Send(msg) //nolint:errcheck
}

func sendAndGet(chatID int64, text string) *tgbotapi.Message {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	sent, err := bot.Send(msg)
	if err != nil {
		log.Printf("[bot] send error: %v", err)
		return nil
	}
	return &sent
}

func edit(chatID int64, msgID int, text string) {
	cfg := tgbotapi.NewEditMessageText(chatID, msgID, text)
	cfg.ParseMode = "HTML"
	cfg.DisableWebPagePreview = true
	if _, err := bot.Send(cfg); err != nil {
		log.Printf("[bot] edit error: %v", err)
	}
}

// ── Active analysis tracking ─────────────────────────────────────

func registerAnalysis(chatID int64, cancel context.CancelFunc) {
	activeMu.Lock()
	defer activeMu.Unlock()
	active[chatID] = cancel
}

func unregisterAnalysis(chatID int64) {
	activeMu.Lock()
	defer activeMu.Unlock()
	delete(active, chatID)
}

func cancelAnalysis(chatID int64) {
	activeMu.Lock()
	defer activeMu.Unlock()
	if cancel, ok := active[chatID]; ok {
		cancel()
		delete(active, chatID)
	}
}

func editWithKeyboard(chatID int64, msgID int, text string, kb tgbotapi.InlineKeyboardMarkup) {
	cfg := tgbotapi.NewEditMessageText(chatID, msgID, text)
	cfg.ParseMode = "HTML"
	cfg.DisableWebPagePreview = true
	cfg.ReplyMarkup = &kb
	if _, err := bot.Send(cfg); err != nil {
		log.Printf("[bot] edit error: %v", err)
	}
}

// ── Video handler ─────────────────────────────────────────────────

func handleVideo(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		send(chatID, "❌ Видеоанализ недоступен: GEMINI_API_KEY не настроен.")
		return
	}

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

	const maxBytes = 50 * 1024 * 1024
	if fileSize == 0 {
		send(chatID, "❌ Не удалось определить размер видео. Пожалуйста, отправьте видео до 50 МБ.")
		return
	}
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
		runVideoAnalysis(ctx, chatID, initMsg.MessageID, fileID, mimeType, geminiKey)
	}()
}

func runVideoAnalysis(ctx context.Context, chatID int64, msgID int, fileID, mimeType, geminiKey string) {
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
	if resp.StatusCode != http.StatusOK {
		edit(chatID, msgID, fmt.Sprintf("❌ <b>Ошибка скачивания видео:</b> HTTP %d", resp.StatusCode))
		return
	}
	videoBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		edit(chatID, msgID, "❌ <b>Ошибка:</b> не удалось прочитать файл.\n<code>"+escHTML(err.Error())+"</code>")
		return
	}

	edit(chatID, msgID, "🎬 <b>Видео получено</b>\n\n<code>Загружаю в Gemini...</code>")
	geminiFile, err := UploadVideoToGemini(ctx, geminiKey, videoBytes, mimeType)
	if err != nil {
		edit(chatID, msgID, "❌ <b>Ошибка загрузки в Gemini:</b>\n<code>"+escHTML(err.Error())+"</code>")
		return
	}
	defer DeleteGeminiFile(geminiKey, geminiFile.Name)

	edit(chatID, msgID, "🎬 <b>Видео получено</b>\n\n<code>Gemini обрабатывает файл...</code>")
	if err := WaitForGeminiFile(ctx, geminiKey, geminiFile.Name); err != nil {
		edit(chatID, msgID, "❌ <b>Ошибка обработки файла:</b>\n<code>"+escHTML(err.Error())+"</code>")
		return
	}

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

	preview := description
	if runes := []rune(preview); len(runes) > 300 {
		preview = string(runes[:300]) + "..."
	}
	edit(chatID, msgID, fmt.Sprintf(
		"🎬 <b>Видео расшифровано</b>\n\n<code>%s</code>\n\n⏳ <b>Анализирую на дезинформацию...</b>",
		escHTML(preview),
	))

	payload := map[string]any{"text": description}
	runAnalysis(ctx, chatID, msgID, payload, "")
}

// ── Callback handler ─────────────────────────────────────────────

func handleCallback(cb *tgbotapi.CallbackQuery) {
	if cb.Data == "" || !strings.HasPrefix(cb.Data, "rescan:") {
		return
	}

	data := strings.TrimPrefix(cb.Data, "rescan:")
	var payload map[string]any

	if strings.HasPrefix(data, "key:") {
		key := strings.TrimPrefix(data, "key:")
		historyMu.Lock()
		payload = history[key]
		historyMu.Unlock()
	} else {
		_ = json.Unmarshal([]byte(data), &payload)
	}

	if payload == nil {
		bot.Send(tgbotapi.NewCallback(cb.ID, "❌ Данные для повторного сканирования не найдены"))
		return
	}

	chatID := cb.Message.Chat.ID
	msgID := cb.Message.MessageID

	// Feedback
	bot.Send(tgbotapi.NewCallback(cb.ID, "🔄 Запускаю повторный анализ..."))

	// Update message back to "Analyzing"
	edit(chatID, msgID, "⏳ <b>Анализирую... (повторно)</b>\n\n<code>Загружаю страницу...</code>")

	cancelAnalysis(chatID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	registerAnalysis(chatID, cancel)

	go func() {
		defer func() {
			cancel()
			unregisterAnalysis(chatID)
		}()
		runAnalysis(ctx, chatID, msgID, payload, "")
	}()
}

// ── Share helper ─────────────────────────────────────────────────

func requestShareURL(result *AnalysisResult) string {
	data, err := json.Marshal(result)
	if err != nil {
		return ""
	}

	resp, err := http.Post(apiBase+"/api/share", "application/json", strings.NewReader(string(data)))
	if err != nil {
		log.Printf("[bot] share error: %v", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var res struct {
		URL string `json:"url"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &res); err != nil {
		return ""
	}

	return res.URL
}

// ── Misc ─────────────────────────────────────────────────────────

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

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
