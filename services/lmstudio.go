package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type LMStudioClient struct {
	BaseURL      string
	Model        string
	PromptConfig *PromptConfig
}

type LMStudioRequest struct {
	Model    string          `json:"model"`
	Messages []LMStudioMessage `json:"messages"`
	Temperature float64      `json:"temperature,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
}

type LMStudioMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LMStudioResponse struct {
	Choices []struct {
		Message LMStudioMessage `json:"message"`
	} `json:"choices"`
}

func NewLMStudioClient(baseURL, model string, promptConfig *PromptConfig) *LMStudioClient {
	return &LMStudioClient{
		BaseURL:      baseURL,
		Model:        model,
		PromptConfig: promptConfig,
	}
}

func (c *LMStudioClient) Analyze(text string) (string, error) {
	log.Printf("[LM STUDIO] 🖥 Локальный анализ с моделью: %s", c.Model)
	log.Printf("[LM STUDIO] 🔗 Подключение к: %s", c.BaseURL)
	
	// Строим системный промпт из конфигурации
	systemPrompt := c.PromptConfig.BuildSystemPrompt()
	
	reqBody := LMStudioRequest{
		Model: c.Model,
		Messages: []LMStudioMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: text},
		},
		Temperature: 0.7,
		MaxTokens:   4000,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("[LM STUDIO] ❌ Ошибка маршалинга: %v", err)
		return "", fmt.Errorf("ошибка маршалинга: %w", err)
	}

	log.Printf("[LM STUDIO] 📊 Размер запроса: %d байт", len(jsonData))
	log.Printf("[LM STUDIO] 📤 Отправляю запрос к LM Studio...")

	req, err := http.NewRequest("POST", c.BaseURL+"/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("[LM STUDIO] ❌ Ошибка создания запроса: %v", err)
		return "", fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	client := &http.Client{
		Timeout: 300 * time.Second, // 5 минут для локальных моделей
	}
	
	log.Printf("[LM STUDIO] ⏳ Ожидаю ответ от локальной модели...")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[LM STUDIO] ❌ Ошибка выполнения запроса: %v", err)
		log.Printf("[LM STUDIO] 💡 Убедитесь, что LM Studio запущен на %s", c.BaseURL)
		return "", fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	elapsed := time.Since(startTime)
	log.Printf("[LM STUDIO] ✓ Получен ответ: статус %d (заняло %.2f сек)", resp.StatusCode, elapsed.Seconds())

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[LM STUDIO] ❌ Ошибка чтения ответа: %v", err)
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	log.Printf("[LM STUDIO] 📊 Размер ответа: %d байт", len(body))

	if resp.StatusCode != http.StatusOK {
		log.Printf("[LM STUDIO] ❌ API вернул ошибку %d: %s", resp.StatusCode, string(body))
		return "", fmt.Errorf("API вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	var lmResp LMStudioResponse
	if err := json.Unmarshal(body, &lmResp); err != nil {
		log.Printf("[LM STUDIO] ❌ Ошибка парсинга ответа: %v", err)
		return "", fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	if len(lmResp.Choices) == 0 {
		log.Printf("[LM STUDIO] ❌ Пустой ответ от API")
		return "", fmt.Errorf("пустой ответ от API")
	}

	responseText := lmResp.Choices[0].Message.Content
	log.Printf("[LM STUDIO] ✓ Успешно получен ответ от локальной модели")
	log.Printf("[LM STUDIO] 📊 Длина ответа: %d символов", len(responseText))
	
	return responseText, nil
}
