package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"text-analyzer/config"
	"text-analyzer/database"
	"text-analyzer/logger"
	"text-analyzer/services"

	"github.com/gorilla/websocket"
)

type AdminHandler struct {
	cfg      *config.Config
	analyzer *services.AnalyzerService
}

func NewAdminHandler(cfg *config.Config, analyzer *services.AnalyzerService) *AdminHandler {
	return &AdminHandler{
		cfg:      cfg,
		analyzer: analyzer,
	}
}

func (h *AdminHandler) Pause(w http.ResponseWriter, r *http.Request) {
	if h.analyzer != nil {
		h.analyzer.IsPaused.Store(true)
		log.Println("[ADMIN] ⏸ Работа бэкенда приостановлена администратором")
	}
	w.WriteHeader(http.StatusOK)
}

func (h *AdminHandler) Resume(w http.ResponseWriter, r *http.Request) {
	if h.analyzer != nil {
		h.analyzer.IsPaused.Store(false)
		log.Println("[ADMIN] ▶ Работа бэкенда возобновлена администратором")
	}
	w.WriteHeader(http.StatusOK)
}

func (h *AdminHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	isPaused := false
	if h.analyzer != nil {
		isPaused = h.analyzer.IsPaused.Load()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"is_paused": isPaused,
	})
}

// AuthMiddleware проверяет токен администратора
func (h *AdminHandler) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Admin-Token")
		if token != h.cfg.AdminToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

type AdminStats struct {
	TotalRequests  int                `json:"total_requests"`
	AverageScore   float64            `json:"average_score"`
	FakeCount      int                `json:"fake_count"`
	RealCount      int                `json:"real_count"`
	RecentRequests []AdminHistoryItem `json:"recent_requests"`
}

type AdminHistoryItem struct {
	ID        int    `json:"id"`
	URL       string `json:"url"`
	Score     int    `json:"score"`
	IsFake    bool   `json:"is_fake"`
	CreatedAt string `json:"created_at"`
}

func (h *AdminHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	if database.DB == nil {
		http.Error(w, "Database not available", http.StatusInternalServerError)
		return
	}

	stats := AdminStats{}

	// Общее количество
	err := database.DB.QueryRow("SELECT COUNT(*) FROM analysis_results").Scan(&stats.TotalRequests)
	if err != nil {
		log.Printf("[ADMIN] Error getting total count: %v", err)
	}

	// Средний балл
	err = database.DB.QueryRow("SELECT COALESCE(AVG((result->>'credibility_score')::int), 0) FROM analysis_results").Scan(&stats.AverageScore)

	// Количество фейков (score <= 5)
	err = database.DB.QueryRow("SELECT COUNT(*) FROM analysis_results WHERE (result->>'credibility_score')::int <= 5").Scan(&stats.FakeCount)
	stats.RealCount = stats.TotalRequests - stats.FakeCount

	// Последние 10 запросов
	rows, err := database.DB.Query(`
		SELECT id, url, (result->>'credibility_score')::int, (result->'verification'->>'is_fake')::boolean, created_at 
		FROM analysis_results 
		ORDER BY created_at DESC 
		LIMIT 10
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			item := AdminHistoryItem{}
			rows.Scan(&item.ID, &item.URL, &item.Score, &item.IsFake, &item.CreatedAt)
			stats.RecentRequests = append(stats.RecentRequests, item)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // В продакшене лучше ограничить
	},
}

func (h *AdminHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token != h.cfg.AdminToken {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ADMIN] WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	logsChan := logger.Instance.Subscribe()
	defer logger.Instance.Unsubscribe(logsChan)

	// Канал для отслеживания закрытия соединения со стороны клиента
	done := make(chan struct{})
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				close(done)
				return
			}
		}
	}()

	for {
		select {
		case msg := <-logsChan:
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}
