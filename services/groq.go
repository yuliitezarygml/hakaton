package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"text-analyzer/models"
	"time"
)

type GroqClient struct {
	APIKey       string
	Model        string
	PromptConfig *PromptConfig
}

func NewGroqClient(apiKey, model string, promptConfig *PromptConfig) *GroqClient {
	return &GroqClient{
		APIKey:       apiKey,
		Model:        model,
		PromptConfig: promptConfig,
	}
}

func (c *GroqClient) Analyze(text string) (string, *models.TokenUsage, error) {
	log.Printf("[GROQ] 🤖 Модель: %s", c.Model)

	systemPrompt := c.PromptConfig.BuildSystemPrompt()

	reqBody := OpenRouterRequest{
		Model: c.Model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: text},
		},
		Temperature: 0.1,
		MaxTokens:   4000,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("ошибка маршалинга: %w", err)
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}

	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			log.Printf("[GROQ] ⏳ Попытка %d/%d, жду 10 секунд...", attempt, maxRetries)
			time.Sleep(10 * time.Second)
		}

		req, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(jsonData))
		if err != nil {
			return "", nil, fmt.Errorf("ошибка создания запроса: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")

		log.Printf("[GROQ] 📤 Отправляю запрос (попытка %d)...", attempt)
		start := time.Now()

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("[GROQ] ❌ Ошибка запроса: %v", err)
			lastErr = err
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		elapsed := time.Since(start)
		log.Printf("[GROQ] ✓ Статус %d (%.2f сек), размер %d байт", resp.StatusCode, elapsed.Seconds(), len(body))

		if resp.StatusCode == 429 {
			log.Printf("[GROQ] ⚠ Rate limit, повторяю...")
			lastErr = fmt.Errorf("rate limit 429")
			continue
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("[GROQ] ❌ Ошибка %d: %s", resp.StatusCode, string(body))
			lastErr = fmt.Errorf("API вернул ошибку %d: %s", resp.StatusCode, string(body))
			continue
		}

		var groqResp OpenRouterResponse
		if err := json.Unmarshal(body, &groqResp); err != nil {
			log.Printf("[GROQ] ❌ Ошибка парсинга: %v", err)
			lastErr = err
			continue
		}

		if len(groqResp.Choices) == 0 {
			log.Printf("[GROQ] ❌ Пустой ответ. Тело: %s", string(body))
			lastErr = fmt.Errorf("пустой ответ от Groq")
			continue
		}

		responseText := groqResp.Choices[0].Message.Content
		
		// Создаем структуру TokenUsage из ответа
		tokenUsage := &models.TokenUsage{
			PromptTokens:     groqResp.Usage.PromptTokens,
			CompletionTokens: groqResp.Usage.CompletionTokens,
			TotalTokens:      groqResp.Usage.TotalTokens,
		}
		
		log.Printf("[GROQ] ✅ Успешно! Длина ответа: %d символов", len(responseText))
		log.Printf("[GROQ] 📊 Токены: %d всего (запрос: %d, ответ: %d)", 
			tokenUsage.TotalTokens, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
		
		return responseText, tokenUsage, nil
	}

	return "", nil, fmt.Errorf("все %d попытки неудачны: %w", maxRetries, lastErr)
}
