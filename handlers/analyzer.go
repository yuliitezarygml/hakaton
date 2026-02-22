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

	sendEvent("start", "🚀 Начинаю анализ...")

	var result *models.AnalysisResponse
	var err error

	if req.URL != "" {
		sendProgress(fmt.Sprintf("🌐 Анализирую URL: %s", req.URL))
		result, err = h.service.AnalyzeURL(req.URL, sendProgress)
	} else {
		sendProgress(fmt.Sprintf("📝 Анализирую текст (%d символов)", len(req.Text)))
		result, err = h.service.AnalyzeText(req.Text, sendProgress)
	}

	if err != nil {
		sendEvent("error", "❌ "+err.Error())
		return
	}

	// Отправляем финальный результат
	resultJSON, _ := json.Marshal(result)
	sendEvent("result", string(resultJSON))
	sendEvent("done", "✅ Анализ завершён!")
}

func (h *AnalyzerHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
