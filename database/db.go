package database

import (
	"database/sql"
	"log"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func InitDB(url string) {
	if url == "" {
		log.Println("⚠️ DB_URL не установлен, работа без базы данных")
		return
	}

	var err error
	DB, err = sql.Open("postgres", url)
	if err != nil {
		log.Fatalf("❌ Ошибка подключения к базе данных: %v", err)
	}

	err = DB.Ping()
	if err != nil {
		log.Fatalf("❌ База данных недоступна: %v", err)
	}

	log.Println("✓ Подключение к PostgreSQL установлено")

	// Создаем таблицу для логов анализа, если она не существует
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS analysis_results (
			id SERIAL PRIMARY KEY,
			text TEXT,
			url TEXT,
			result JSONB,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatalf("❌ Ошибка создания таблицы: %v", err)
	}

	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS domain_stats (
			domain TEXT PRIMARY KEY,
			total_analyses INTEGER DEFAULT 0,
			sum_scores     INTEGER DEFAULT 0,
			avg_score      FLOAT   DEFAULT 0,
			last_analyzed_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		log.Fatalf("❌ Ошибка создания таблицы domain_stats: %v", err)
	}

	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS shared_results (
			id         TEXT PRIMARY KEY,
			result     JSONB NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			expires_at TIMESTAMPTZ DEFAULT NOW() + INTERVAL '30 days'
		)
	`)
	if err != nil {
		log.Fatalf("❌ Ошибка создания таблицы shared_results: %v", err)
	}
}
