package services

import (
	"log"
	"net/url"
	"strings"
	"text-analyzer/database"
)

// NormalizeDomain extracts host from a URL and strips www. prefix and port.
func NormalizeDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}

// UpsertDomainStats updates domain reputation after each URL analysis.
func UpsertDomainStats(rawURL string, score int) {
	if database.DB == nil {
		return
	}
	domain := NormalizeDomain(rawURL)
	if domain == "" {
		return
	}
	_, err := database.DB.Exec(`
		INSERT INTO domain_stats (domain, total_analyses, sum_scores, avg_score, last_analyzed_at)
		VALUES ($1, 1, $2::INTEGER, $2::FLOAT, NOW())
		ON CONFLICT (domain) DO UPDATE SET
			total_analyses   = domain_stats.total_analyses + 1,
			sum_scores       = domain_stats.sum_scores + $2::INTEGER,
			avg_score        = (domain_stats.sum_scores + $2)::float / (domain_stats.total_analyses + 1),
			last_analyzed_at = NOW()
	`, domain, score)
	if err != nil {
		log.Printf("[DOMAIN] ⚠ Ошибка обновления stats для %s: %v", domain, err)
	} else {
		log.Printf("[DOMAIN] ✓ Stats обновлены: %s score=%d", domain, score)
	}
}
