package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"text-analyzer/models"
	"text-analyzer/services"
	"time"
)

type AnalyzerHandler struct {
	service *services.AnalyzerService
}

func NewAnalyzerHandler(service *services.AnalyzerService) *AnalyzerHandler {
	return &AnalyzerHandler{service: service}
}

// Analyze — обычный endpoint, возвращает финальный JSON
func (h *AnalyzerHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	// CORS заголовки
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Обработка preflight запроса
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Проверяем, не приостановлен ли анализатор
	if h.service != nil && h.service.IsPaused.Load() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "backend_paused", "message": "Анализатор приостановлен администратором"})
		log.Printf("[HANDLER] ⏸ Запрос отклонён — анализатор приостановлен")
		return
	}

	startTime := time.Now()
	log.Printf("\n========================================")
	log.Printf("[HANDLER] 📥 Получен запрос: %s %s", r.Method, r.RemoteAddr)

	if r.Method != http.MethodPost {
		http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}

	var req models.AnalysisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Неверный формат запроса", http.StatusBadRequest)
		return
	}

	var result *models.AnalysisResponse
	var err error

	if req.URL != "" {
		log.Printf("[HANDLER] 🌐 Анализ URL: %s", req.URL)
		result, err = h.service.AnalyzeURL(req.URL)
	} else if req.Text != "" {
		log.Printf("[HANDLER] 📝 Анализ текста (%d символов)", len(req.Text))
		result, err = h.service.AnalyzeText(req.Text)
	} else {
		http.Error(w, "Необходимо указать 'text' или 'url'", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Ошибка анализа: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[HANDLER] ✅ Готово за %v", time.Since(startTime))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(result)
}

// AnalyzeStream — SSE endpoint, показывает прогресс в реальном времени
func (h *AnalyzerHandler) AnalyzeStream(w http.ResponseWriter, r *http.Request) {
	// Проверяем, не приостановлен ли анализатор
	if h.service != nil && h.service.IsPaused.Load() {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		fmt.Fprintf(w, "event: error\ndata: Анализатор приостановлен администратором\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		log.Printf("[HANDLER] ⏸ Stream-запрос отклонён — анализатор приостановлен")
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}

	var req models.AnalysisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Неверный формат запроса", http.StatusBadRequest)
		return
	}

	if req.URL == "" && req.Text == "" {
		http.Error(w, "Необходимо указать 'text' или 'url'", http.StatusBadRequest)
		return
	}

	// SSE заголовки
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming не поддерживается", http.StatusInternalServerError)
		return
	}

	// Функция отправки SSE события
	sendEvent := func(eventType, data string) {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
		flusher.Flush()
	}

	sendProgress := func(msg string) {
		sendEvent("progress", msg)
	}

	sendEvent("start", "🚀 Начинаю проверку...")

	var result *models.AnalysisResponse
	var err error

	if req.URL != "" {
		result, err = h.service.AnalyzeURL(req.URL, sendProgress)
	} else {
		sendProgress(fmt.Sprintf("📄 Текст получен (%d символов), начинаю проверку...", len(req.Text)))
		result, err = h.service.AnalyzeText(req.Text, sendProgress)
	}

	if err != nil {
		sendEvent("error", "❌ "+err.Error())
		return
	}

	resultJSON, _ := json.Marshal(result)
	sendEvent("result", string(resultJSON))
	sendEvent("done", "✅ Проверка завершена!")
}

func (h *AnalyzerHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Limits — возвращает текущее состояние rate limits по всем провайдерам
func (h *AnalyzerHandler) Limits(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(services.GetRateLimits())
}

// Chat — endpoint для общения с AI на основе результатов анализа
func (h *AnalyzerHandler) Chat(w http.ResponseWriter, r *http.Request) {
	// CORS заголовки
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Обработка preflight запроса
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("[HANDLER] 💬 Получен запрос к чату: %s", r.RemoteAddr)

	var req models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Неверный формат запроса", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "Сообщение не может быть пустым", http.StatusBadRequest)
		return
	}

	log.Printf("[HANDLER] 📝 Вопрос: %s", req.Message)

	result, err := h.service.Chat(req.Message, req.AnalysisContext)
	if err != nil {
		log.Printf("[HANDLER] ❌ Ошибка: %v", err)
		http.Error(w, "Ошибка обработки запроса: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[HANDLER] 💬 Ответ AI: %s", result.Response)
	log.Printf("[HANDLER] ✅ Ответ отправлен")

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(result)
}
