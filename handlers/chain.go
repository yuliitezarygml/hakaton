package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"text-analyzer/services"
)

// ChainHandler обрабатывает запросы на построение цепочки источников.
type ChainHandler struct {
	service *services.ChainService
}

func NewChainHandler(service *services.ChainService) *ChainHandler {
	return &ChainHandler{service: service}
}

// Stream — SSE endpoint POST /api/chain/stream.
// Принимает { "url": "..." }, стримит chain_* события.
func (h *ChainHandler) Stream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, "Необходимо указать 'url'", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming не поддерживается", http.StatusInternalServerError)
		return
	}

	sendSSE := func(eventType, data string) {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
		flusher.Flush()
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	log.Printf("[CHAIN] 🔗 Запрос цепочки для URL: %s", req.URL)

	emit := func(ev services.ChainEvent) {
		data, _ := json.Marshal(ev)
		sendSSE(ev.Type, string(data))
	}

	if err := h.service.BuildChain(ctx, req.URL, emit); err != nil {
		log.Printf("[CHAIN] ❌ Ошибка: %v", err)
		errData, _ := json.Marshal(map[string]string{"message": err.Error()})
		sendSSE("chain_error", string(errData))
	}
}
