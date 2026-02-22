package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	OpenRouterAPIKey      string
	OpenRouterModel       string
	OpenRouterModelBackup string
	UseGroq               bool
	GroqAPIKey            string
	GroqModel             string
	UseLMStudio           bool
	LMStudioURL           string
	LMStudioModel         string
	SerperAPIKey          string
	Port                  string
}

func Load() (*Config, error) {
	godotenv.Load()

	modelBackup := os.Getenv("OPENROUTER_MODEL_BACKUP")
	useLMStudio := os.Getenv("USE_LM_STUDIO") == "true"
	useGroq := os.Getenv("USE_GROQ") == "true"

	return &Config{
		OpenRouterAPIKey:      os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel:       getEnvOrDefault("OPENROUTER_MODEL", "nvidia/nemotron-3-nano-30b-a3b:free"),
		OpenRouterModelBackup: modelBackup,
		UseGroq:               useGroq,
		GroqAPIKey:            os.Getenv("GROQ_API_KEY"),
		GroqModel:             getEnvOrDefault("GROQ_MODEL", "llama-3.3-70b-versatile"),
		UseLMStudio:           useLMStudio,
		LMStudioURL:           getEnvOrDefault("LM_STUDIO_URL", "http://localhost:1234"),
		LMStudioModel:         getEnvOrDefault("LM_STUDIO_MODEL", "local-model"),
		SerperAPIKey:          os.Getenv("SERPER_API_KEY"),
		Port:                  getEnvOrDefault("PORT", "8080"),
	}, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
