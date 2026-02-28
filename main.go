package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"text-analyzer/cache"
	"text-analyzer/config"
	"text-analyzer/database"
	"text-analyzer/handlers"
	"text-analyzer/logger"
	"text-analyzer/services"
)

func main() {
	log.SetOutput(logger.GetWriter())
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("🚀 Запуск Text Analyzer...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("❌ Ошибка загрузки конфигурации:", err)
	}

	log.Printf("✓ Конфигурация загружена")

	database.InitDB(cfg.DbUrl)
	cache.InitRedis(cfg.RedisUrl)

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

	var factCheckClient *services.GoogleFactCheckClient
	if cfg.GoogleFactCheckAPIKey != "" {
		factCheckClient = services.NewGoogleFactCheckClient(cfg.GoogleFactCheckAPIKey)
		log.Printf("✓ Google Fact Check клиент инициализирован")
	} else {
		log.Printf("  - Google Fact Check API: отключен")
	}

	var analyzerService *services.AnalyzerService

	switch {
	case cfg.UseGroq:
		log.Println("⚡ Инициализация Groq клиента...")
		groqClient := services.NewGroqClient(cfg.GroqAPIKeys, cfg.GroqModel, promptConfig)
		analyzerService = services.NewAnalyzerService(groqClient, contentFetcher, serperClient, factCheckClient, promptConfig)
		log.Println("✓ Groq режим активирован")

	default:
		if cfg.OpenRouterAPIKey == "" {
			log.Fatal("❌ OPENROUTER_API_KEY не установлен")
		}
		log.Println("☁ Инициализация OpenRouter клиента...")
		openRouterClient := services.NewOpenRouterClient(cfg.OpenRouterAPIKey, cfg.OpenRouterModel, cfg.OpenRouterModelBackup, promptConfig)
		analyzerService = services.NewAnalyzerService(openRouterClient, contentFetcher, serperClient, factCheckClient, promptConfig)
		log.Println("✓ OpenRouter режим активирован")
	}

	chainService := services.NewChainService(
		func() services.AIClient {
			switch {
			case cfg.UseGroq:
				return services.NewGroqClient(cfg.GroqAPIKeys, cfg.GroqModel, promptConfig)
			default:
				return services.NewOpenRouterClient(cfg.OpenRouterAPIKey, cfg.OpenRouterModel, cfg.OpenRouterModelBackup, promptConfig)
			}
		}(),
		contentFetcher,
		serperClient,
	)

	analyzerHandler := handlers.NewAnalyzerHandler(analyzerService)
	chainHandler := handlers.NewChainHandler(chainService)
	domainHandler := handlers.NewDomainHandler()
	shareHandler := handlers.NewShareHandler()
	adminHandler := handlers.NewAdminHandler(cfg, analyzerService)
	dockerHandler := handlers.NewDockerHandler(adminHandler)
	log.Println("✓ Сервисы инициализированы")

	http.HandleFunc("/api/chain/stream", chainHandler.Stream)
	http.HandleFunc("/api/ext/hash", analyzerHandler.ExtHash)
	http.HandleFunc("/api/analyze", analyzerHandler.Analyze)
	http.HandleFunc("/api/analyze/stream", analyzerHandler.AnalyzeStream)
	http.HandleFunc("/api/chat", analyzerHandler.Chat)
	http.HandleFunc("/api/health", analyzerHandler.Health)
	http.HandleFunc("/api/limits", analyzerHandler.Limits)
	http.HandleFunc("/api/domain/", domainHandler.GetDomain)
	http.HandleFunc("/api/domains/top", domainHandler.GetTopDomains)
	http.HandleFunc("/api/share", shareHandler.Create)
	http.HandleFunc("/api/share/", shareHandler.GetResult)
	http.HandleFunc("/s/", shareHandler.ShowPage)

	// Admin API
	http.HandleFunc("/api/admin/stats", adminHandler.AuthMiddleware(adminHandler.GetStats))
	http.HandleFunc("/api/admin/logs", adminHandler.StreamLogs)
	http.HandleFunc("/api/admin/pause", adminHandler.AuthMiddleware(adminHandler.Pause))
	http.HandleFunc("/api/admin/resume", adminHandler.AuthMiddleware(adminHandler.Resume))
	http.HandleFunc("/api/admin/status", adminHandler.AuthMiddleware(adminHandler.GetStatus))

	// Docker management API
	http.HandleFunc("/api/admin/docker/containers", adminHandler.AuthMiddleware(dockerHandler.ListContainers))
	http.HandleFunc("/api/admin/docker/action", adminHandler.AuthMiddleware(dockerHandler.ContainerAction))
	http.HandleFunc("/api/admin/docker/logs", dockerHandler.StreamContainerLogs)

	// Статические файлы админки
	fs := http.FileServer(http.Dir("admin"))
	http.Handle("/admin/", http.StripPrefix("/admin/", fs))

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
