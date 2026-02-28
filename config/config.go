package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	OpenRouterAPIKey      string
	OpenRouterModel       string
	OpenRouterModelBackup string
	UseGroq               bool
	GroqAPIKeys           []string
	GroqModel             string
	SerperAPIKey          string
	GoogleFactCheckAPIKey string
	Port                  string
	DbUrl                 string
	RedisUrl              string
	AdminToken            string
}

func Load() (*Config, error) {
	godotenv.Load()

	modelBackup := os.Getenv("OPENROUTER_MODEL_BACKUP")
	useGroq := os.Getenv("USE_GROQ") == "true"

	var groqKeys []string
	// Load GROQ_API_KEY, GROQ_API_KEY2, ..., GROQ_API_KEY7
	mainKey := os.Getenv("GROQ_API_KEY")
	if mainKey != "" {
		groqKeys = append(groqKeys, mainKey)
	}
	for i := 2; i <= 7; i++ {
		key := os.Getenv(fmt.Sprintf("GROQ_API_KEY%d", i))
		if key != "" {
			groqKeys = append(groqKeys, key)
		}
	}

	return &Config{
		OpenRouterAPIKey:      os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel:       getEnvOrDefault("OPENROUTER_MODEL", "nvidia/nemotron-3-nano-30b-a3b:free"),
		OpenRouterModelBackup: modelBackup,
		UseGroq:               useGroq,
		GroqAPIKeys:           groqKeys,
		GroqModel:             getEnvOrDefault("GROQ_MODEL", "llama-3.3-70b-versatile"),
		SerperAPIKey:          os.Getenv("SERPER_API_KEY"),
		GoogleFactCheckAPIKey: os.Getenv("GOOGLE_FACT_CHECK_API_KEY"),
		Port:                  getEnvOrDefault("PORT", "8080"),
		DbUrl:                 os.Getenv("DB_URL"),
		RedisUrl:              os.Getenv("REDIS_URL"),
		AdminToken:            getEnvOrDefault("ADMIN_TOKEN", "admin_secret_123"),
	}, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
