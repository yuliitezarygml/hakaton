package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"text-analyzer/database"
)

type ShareHandler struct {
	baseURL string
}

func NewShareHandler() *ShareHandler {
	base := os.Getenv("API_BASE")
	if base == "" {
		base = "http://localhost:8080"
	}
	return &ShareHandler{baseURL: base}
}

func newShareID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Create — POST /api/share → {"id":"…","url":"…"}
func (h *ShareHandler) Create(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if database.DB == nil {
		http.Error(w, `{"error":"db unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	id := newShareID()
	_, err := database.DB.Exec(
		`INSERT INTO shared_results (id, result) VALUES ($1, $2)`,
		id, []byte(raw),
	)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	shareURL := h.baseURL + "/s/" + id
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "url": shareURL})
}

// GetResult — GET /api/share/:id → raw JSON result
func (h *ShareHandler) GetResult(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	id := strings.TrimPrefix(r.URL.Path, "/api/share/")
	if id == "" || database.DB == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var raw []byte
	err := database.DB.QueryRow(
		`SELECT result FROM shared_results WHERE id = $1 AND expires_at > NOW()`,
		id,
	).Scan(&raw)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "не найдено или истёк срок"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

// ShowPage — GET /s/:id → serves admin/share.html
func (h *ShareHandler) ShowPage(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile("admin/share.html")
	if err != nil {
		http.Error(w, "share page not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}
