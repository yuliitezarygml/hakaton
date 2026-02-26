package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"text-analyzer/config"
	"text-analyzer/handlers"
	"text-analyzer/services"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("🚀 Запуск Text Analyzer...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("❌ Ошибка загрузки конфигурации:", err)
	}

	log.Printf("✓ Конфигурация загружена")
	if cfg.UseGroq {
		log.Printf("  - Режим: Groq ⚡")
		log.Printf("  - Модель: %s", cfg.GroqModel)
	} else {
		log.Printf("  - Режим: OpenRouter ☁")
		log.Printf("  - Модель 1: %s", cfg.OpenRouterModel)
		if cfg.OpenRouterModelBackup != "" {
			log.Printf("  - Модель 2: %s", cfg.OpenRouterModelBackup)
		}
	}
	log.Printf("  - Порт: %s", cfg.Port)
	if cfg.SerperAPIKey != "" {
		log.Printf("  - Serper API: включен ✓")
	} else {
		log.Printf("  - Serper API: отключен")
	}

	promptConfig, err := services.LoadPromptConfig("config/prompts.json")
	if err != nil {
		log.Fatal("❌ Ошибка загрузки промпт конфигурации:", err)
	}
	log.Printf("✓ Промпт конфигурация загружена")

	contentFetcher := services.NewContentFetcher()

	var serperClient *services.SerperClient
	if cfg.SerperAPIKey != "" {
		serperClient = services.NewSerperClient(cfg.SerperAPIKey)
		log.Printf("✓ Serper клиент инициализирован")
	}

	var analyzerService *services.AnalyzerService

	switch {
	case cfg.UseGroq:
		log.Println("⚡ Инициализация Groq клиента...")
		groqClient := services.NewGroqClient(cfg.GroqAPIKey, cfg.GroqModel, promptConfig)
		analyzerService = services.NewAnalyzerService(groqClient, contentFetcher, serperClient, promptConfig)
		log.Println("✓ Groq режим активирован")

	default:
		if cfg.OpenRouterAPIKey == "" {
			log.Fatal("❌ OPENROUTER_API_KEY не установлен")
		}
		log.Println("☁ Инициализация OpenRouter клиента...")
		openRouterClient := services.NewOpenRouterClient(cfg.OpenRouterAPIKey, cfg.OpenRouterModel, cfg.OpenRouterModelBackup, promptConfig)
		analyzerService = services.NewAnalyzerService(openRouterClient, contentFetcher, serperClient, promptConfig)
		log.Println("✓ OpenRouter режим активирован")
	}

	analyzerHandler := handlers.NewAnalyzerHandler(analyzerService)
	log.Println("✓ Сервисы инициализированы")

	http.HandleFunc("/api/analyze", analyzerHandler.Analyze)
	http.HandleFunc("/api/analyze/stream", analyzerHandler.AnalyzeStream)
	http.HandleFunc("/api/chat", analyzerHandler.Chat)
	http.HandleFunc("/api/health", analyzerHandler.Health)

	addr := ":" + cfg.Port
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Printf("🎯 Сервер запущен на http://localhost%s\n", addr)
	if cfg.UseGroq {
		fmt.Printf("⚡ Режим: Groq | Модель: %s\n", cfg.GroqModel)
	} else {
		fmt.Printf("☁ Режим: OpenRouter | Модель: %s\n", cfg.OpenRouterModel)
	}
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println("\n📝 Примеры:")
	fmt.Printf(`   curl -X POST http://localhost%s/api/analyze -H "Content-Type: application/json" -d '{"text": "текст"}'`+"\n", addr)
	fmt.Printf(`   curl -X POST http://localhost%s/api/analyze -H "Content-Type: application/json" -d '{"url": "https://..."}'`+"\n", addr)
	fmt.Println("\n" + strings.Repeat("=", 50) + "\n")

	log.Println("✓ Сервер готов к приему запросов...")
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("❌ Ошибка запуска сервера:", err)
	}
}
