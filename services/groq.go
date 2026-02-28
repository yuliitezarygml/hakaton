package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"text-analyzer/models"
	"time"
)

type GroqClient struct {
	APIKeys      []string
	Model        string
	PromptConfig *PromptConfig
	mu           sync.Mutex
	currentIndex int
}

func NewGroqClient(apiKeys []string, model string, promptConfig *PromptConfig) *GroqClient {
	return &GroqClient{
		APIKeys:      apiKeys,
		Model:        model,
		PromptConfig: promptConfig,
	}
}

func (c *GroqClient) getAPIKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.APIKeys) == 0 {
		return ""
	}
	return c.APIKeys[c.currentIndex]
}

func (c *GroqClient) rotateKey() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.APIKeys) <= 1 {
		return
	}
	c.currentIndex = (c.currentIndex + 1) % len(c.APIKeys)
	log.Printf("[GROQ] 🔄 Переключение на ключ #%d", c.currentIndex+1)
}

func (c *GroqClient) Analyze(text string) (string, *models.TokenUsage, error) {
	log.Printf("[GROQ] 🤖 Модель: %s (Ключей доступно: %d)", c.Model, len(c.APIKeys))

	// Ограничиваем текст ~6000 токенов (~24000 символов)
	// чтобы не превышать лимит 12000 TPM с учётом системного промпта
	const maxRunes = 24000
	runes := []rune(text)
	if len(runes) > maxRunes {
		log.Printf("[GROQ] ✂ Текст обрезан с %d до %d символов (лимит токенов)", len(runes), maxRunes)
		text = string(runes[:maxRunes]) + "\n\n[...контент обрезан для соблюдения лимита токенов...]"
	}

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

	maxRetries := len(c.APIKeys)
	if maxRetries < 3 {
		maxRetries = 3
	}
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		apiKey := c.getAPIKey()

		req, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(jsonData))
		if err != nil {
			return "", nil, fmt.Errorf("ошибка создания запроса: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		log.Printf("[GROQ] 📤 Отправляю запрос (попытка %d, ключ #%d)...", attempt, c.currentIndex+1)
		start := time.Now()

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("[GROQ] ❌ Ошибка запроса: %v", err)
			lastErr = err
			c.rotateKey()
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		elapsed := time.Since(start)
		log.Printf("[GROQ] ✓ Статус %d (%.2f сек), размер %d байт", resp.StatusCode, elapsed.Seconds(), len(body))

		// Capture rate limit headers from every response
		UpdateRateLimit("groq", resp, resp.StatusCode)

		if resp.StatusCode == 429 {
			waitSec := 60 // default
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				fmt.Sscanf(ra, "%d", &waitSec)
			} else if ra := resp.Header.Get("X-RateLimit-Reset-Requests"); ra != "" {
				if d, err := time.ParseDuration(ra); err == nil {
					waitSec = int(d.Seconds()) + 1
				}
			}
			log.Printf("[GROQ] ⚠ Rate limit 429 на ключе #%d — лимит исчерпан. Ротация ключа.", c.currentIndex+1)
			c.rotateKey()
			lastErr = fmt.Errorf("лимит запросов исчерпан (429) на текущем ключе")
			continue
		}

		if resp.StatusCode == 413 {
			log.Printf("[GROQ] ❌ Запрос слишком большой (413) — ключи вращать бесполезно")
			return "", nil, fmt.Errorf("запрос слишком большой для модели (413): уменьшите размер текста")
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("[GROQ] ❌ Ошибка %d: %s", resp.StatusCode, string(body))
			lastErr = fmt.Errorf("API вернул ошибку %d: %s", resp.StatusCode, string(body))
			c.rotateKey()
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
