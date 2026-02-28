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
	}

	// Only URL analysis is supported
	if isURL(text) {
		payload := map[string]any{"url": text}
		startAnalysisForChat(chatID, payload, "")
	} else {
		send(chatID, textNotSupportedMsg())
	}
}

// ── Forwarded message handler ─────────────────────────────────────

func handleForwarded(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	// Extract source name
	sourceName := ""
	sourceLink := ""

	switch {
	case msg.ForwardFromChat != nil:
		chat := msg.ForwardFromChat
		if chat.Title != "" {
			sourceName = chat.Title
		}
		if chat.UserName != "" {
			sourceLink = "https://t.me/" + chat.UserName
		}
	case msg.ForwardFrom != nil:
		u := msg.ForwardFrom
		if u.UserName != "" {
			sourceName = "@" + u.UserName
			sourceLink = "https://t.me/" + u.UserName
		} else {
			sourceName = strings.TrimSpace(u.FirstName + " " + u.LastName)
		}
	default:
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

	// Build payload — only URL is supported
	var payload map[string]any
	if detectedURL != "" && isURL(detectedURL) {
		payload = map[string]any{"url": detectedURL}
	} else if isURL(text) {
		payload = map[string]any{"url": text}
	} else {
		// No URL found — politely inform
		msg2 := "🔄 <b>Пересланное сообщение получено</b>"
		if sourceName != "" {
			if sourceLink != "" {
				msg2 += fmt.Sprintf("\n📢 Источник: <a href=\"%s\">%s</a>", sourceLink, escHTML(sourceName))
			} else {
				msg2 += fmt.Sprintf("\n📢 Источник: <b>%s</b>", escHTML(sourceName))
			}
		}
		msg2 += "\n\n" + textNotSupportedMsg()
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
		shareURL := requestShareURL(finalResult)
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

func textNotSupportedMsg() string {
	return `🙏 <b>Пожалуйста, отправьте ссылку на статью или новость.</b>

Анализ текста пока находится в разработке. Бот пока умеет проверять только <b>URL</b>-адреса.

<b>Пример:</b>
<code>https://example.com/article</code>

Как только анализ текста будет готов — мы сразу вас уведомим! 🚀`
}

func startText() string {
	return `🔍 <b>Text Analyzer Bot</b>

Я анализирую статьи и новости на предмет <b>дезинформации</b>, манипуляций и логических ошибок.

<b>Как использовать:</b>
• Отправьте <b>URL</b> статьи — и я её проанализирую
• <b>Перешлите</b> сообщение из канала или чата с ссылкой

<b>Команды:</b>
/cancel — остановить текущий анализ
/help — помощь`
}

func helpText() string {
	return `📖 <b>Помощь</b>

<b>Отправить URL статьи:</b>
<code>https://example.com/article</code>

<b>Переслать сообщение из канала:</b>
Перешлите любое сообщение содержащее ссылку — бот автоматически обнаружит источник и проверит статью.

<b>Результат включает:</b>
• Балл достоверности (0–10)
• Вердикт (достоверно / сомнительно / дезинформация)
• Краткое резюме
• Список манипуляций
• Логические ошибки
• Утверждения без доказательств

<b>Команды:</b>
/cancel — остановить анализ
/start — главное меню`
}
