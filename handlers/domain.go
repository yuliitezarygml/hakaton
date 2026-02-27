package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"text-analyzer/database"
	"text-analyzer/services"
)

type DomainHandler struct{}

func NewDomainHandler() *DomainHandler { return &DomainHandler{} }

type DomainStats struct {
	Domain         string  `json:"domain"`
	TotalAnalyses  int     `json:"total_analyses"`
	AvgScore       float64 `json:"avg_score"`
	Verdict        string  `json:"verdict"`
	LastAnalyzedAt string  `json:"last_analyzed_at"`
}

func verdictFromScore(avg float64) string {
	switch {
	case avg >= 7:
		return "надёжный"
	case avg >= 4:
		return "сомнительный"
	default:
		return "ненадёжный"
	}
}

func domainCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
}

// GetDomain — GET /api/domain/<domain>
func (h *DomainHandler) GetDomain(w http.ResponseWriter, r *http.Request) {
	domainCORSHeaders(w)
	raw := strings.TrimPrefix(r.URL.Path, "/api/domain/")
	domain := services.NormalizeDomain("https://" + raw)
	if domain == "" || database.DB == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}

	var s DomainStats
	err := database.DB.QueryRow(`
		SELECT domain, total_analyses, avg_score, last_analyzed_at
		FROM domain_stats WHERE domain = $1
	`, domain).Scan(&s.Domain, &s.TotalAnalyses, &s.AvgScore, &s.LastAnalyzedAt)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "домен не найден"})
		return
	}
	s.Verdict = verdictFromScore(s.AvgScore)
	json.NewEncoder(w).Encode(s)
}

// GetTopDomains — GET /api/domains/top
func (h *DomainHandler) GetTopDomains(w http.ResponseWriter, r *http.Request) {
	domainCORSHeaders(w)
	if database.DB == nil {
		json.NewEncoder(w).Encode([]DomainStats{})
		return
	}

	rows, err := database.DB.Query(`
		SELECT domain, total_analyses, avg_score, last_analyzed_at
		FROM domain_stats
		ORDER BY total_analyses DESC
		LIMIT 20
	`)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var list []DomainStats
	for rows.Next() {
		var s DomainStats
		rows.Scan(&s.Domain, &s.TotalAnalyses, &s.AvgScore, &s.LastAnalyzedAt)
		s.Verdict = verdictFromScore(s.AvgScore)
		list = append(list, s)
	}
	if list == nil {
		list = []DomainStats{}
	}
	json.NewEncoder(w).Encode(list)
}
