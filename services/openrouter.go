package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"text-analyzer/models"
	"time"
)

type OpenRouterClient struct {
	APIKey        string
	Model         string
	ModelBackup   string
	PromptConfig  *PromptConfig
}

type OpenRouterRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenRouterResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func NewOpenRouterClient(apiKey, model, modelBackup string, promptConfig *PromptConfig) *OpenRouterClient {
	return &OpenRouterClient{
		APIKey:       apiKey,
		Model:        model,
		ModelBackup:  modelBackup,
		PromptConfig: promptConfig,
	}
}

func (c *OpenRouterClient) Analyze(text string) (string, *models.TokenUsage, error) {
	hasBackup := c.ModelBackup != "" && c.ModelBackup != c.Model

	// Пробуем основную модель
	log.Printf("[OPENROUTER] 🤖 Основная модель: %s", c.Model)
	response, usage, err := c.analyzeWithModel(text, c.Model)
	if err == nil {
		log.Printf("[OPENROUTER] ✅ Основная модель ответила успешно")
		return response, usage, nil
	}

	log.Printf("[OPENROUTER] ⚠ Основная модель недоступна: %v", err)

	// Если есть резервная — пробуем её
	if hasBackup {
		log.Printf("[OPENROUTER] 🔄 Переключаюсь на резервную модель: %s", c.ModelBackup)
		response, usage, err = c.analyzeWithModel(text, c.ModelBackup)
		if err == nil {
			log.Printf("[OPENROUTER] ✅ Резервная модель ответила успешно")
			return response, usage, nil
		}
		log.Printf("[OPENROUTER] ❌ Резервная модель тоже недоступна: %v", err)
		return "", nil, fmt.Errorf("обе модели недоступны: %w", err)
	}

	return "", nil, err
}

func (c *OpenRouterClient) analyzeWithModel(text, model string) (string, *models.TokenUsage, error) {
	log.Printf("[OPENROUTER] Подготовка запроса к модели: %s", model)

	systemPrompt := c.PromptConfig.BuildSystemPrompt()

	reqBody := OpenRouterRequest{
		Model: model,
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

	httpClient := &http.Client{Timeout: 90 * time.Second}

	// Retry-цикл: 3 попытки с паузой при 429
	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			log.Printf("[OPENROUTER] ⏳ Попытка %d/%d, жду 15 секунд...", attempt, maxRetries)
			time.Sleep(15 * time.Second)
		}

		req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(jsonData))
		if err != nil {
			return "", nil, fmt.Errorf("ошибка создания запроса: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("HTTP-Referer", "https://text-analyzer.local")
		req.Header.Set("X-Title", "Text Analyzer")

		log.Printf("[OPENROUTER] 📤 Запрос к %s (попытка %d)...", model, attempt)
		startTime := time.Now()

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("[OPENROUTER] ❌ Ошибка запроса: %v", err)
			lastErr = fmt.Errorf("ошибка выполнения запроса: %w", err)
			continue
		}

		elapsed := time.Since(startTime)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		log.Printf("[OPENROUTER] ✓ Статус %d (%.2f сек), размер %d байт", resp.StatusCode, elapsed.Seconds(), len(body))

		if resp.StatusCode == 429 {
			log.Printf("[OPENROUTER] ⚠ Rate limit, повторяю...")
			lastErr = fmt.Errorf("rate limit 429")
			continue
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("[OPENROUTER] ❌ Ошибка %d: %s", resp.StatusCode, string(body))
			lastErr = fmt.Errorf("API вернул ошибку %d: %s", resp.StatusCode, string(body))
			continue
		}

		var openRouterResp OpenRouterResponse
		if err := json.Unmarshal(body, &openRouterResp); err != nil {
			log.Printf("[OPENROUTER] ❌ Ошибка парсинга: %v", err)
			lastErr = fmt.Errorf("ошибка парсинга ответа: %w", err)
			continue
		}

		if len(openRouterResp.Choices) == 0 {
			log.Printf("[OPENROUTER] ❌ Пустой ответ. Тело: %s", string(body))
			lastErr = fmt.Errorf("пустой ответ от API")
			continue
		}

		responseText := openRouterResp.Choices[0].Message.Content
		
		// Создаем структуру TokenUsage из ответа
		tokenUsage := &models.TokenUsage{
			PromptTokens:     openRouterResp.Usage.PromptTokens,
			CompletionTokens: openRouterResp.Usage.CompletionTokens,
			TotalTokens:      openRouterResp.Usage.TotalTokens,
		}
		
		log.Printf("[OPENROUTER] ✅ Успешно! Длина ответа: %d символов", len(responseText))
		log.Printf("[OPENROUTER] 📊 Токены: %d всего (запрос: %d, ответ: %d)", 
			tokenUsage.TotalTokens, tokenUsage.PromptTokens, tokenUsage.CompletionTokens)
		
		return responseText, tokenUsage, nil
	}

	return "", nil, fmt.Errorf("все %d попытки неудачны: %w", maxRetries, lastErr)
}


// combineResponses объединяет результаты двух моделей для максимальной точности
func (c *OpenRouterClient) combineResponses(response1, response2 string) string {
	log.Printf("[OPENROUTER] 🔍 Анализирую ответы обеих моделей...")
	
	// Парсим оба JSON ответа
	var data1, data2 map[string]interface{}
	
	json1 := extractJSONFromResponse(response1)
	json2 := extractJSONFromResponse(response2)
	
	err1 := json.Unmarshal([]byte(json1), &data1)
	err2 := json.Unmarshal([]byte(json2), &data2)
	
	// Если оба не распарсились, возвращаем первый
	if err1 != nil && err2 != nil {
		log.Printf("[OPENROUTER] ⚠ Не удалось распарсить ответы, возвращаю первый")
		return response1
	}
	
	// Если один не распарсился, возвращаем другой
	if err1 != nil {
		log.Printf("[OPENROUTER] ⚠ Модель 1 вернула невалидный JSON, использую модель 2")
		return response2
	}
	if err2 != nil {
		log.Printf("[OPENROUTER] ⚠ Модель 2 вернула невалидный JSON, использую модель 1")
		return response1
	}
	
	// Объединяем данные
	combined := make(map[string]interface{})
	
	// Summary - берем более длинное
	summary1 := getString(data1, "summary")
	summary2 := getString(data2, "summary")
	if len(summary1) > len(summary2) {
		combined["summary"] = summary1
	} else {
		combined["summary"] = summary2
	}
	
	// Credibility score - берем минимум (консервативный подход: если одна модель видит проблему - это важно)
	score1 := getInt(data1, "credibility_score")
	score2 := getInt(data2, "credibility_score")
	minScore := score1
	if score2 < minScore {
		minScore = score2
	}
	combined["credibility_score"] = minScore
	
	// Reasoning - объединяем
	reasoning1 := getString(data1, "reasoning")
	reasoning2 := getString(data2, "reasoning")
	combined["reasoning"] = fmt.Sprintf("МОДЕЛЬ 1: %s\n\nМОДЕЛЬ 2: %s", reasoning1, reasoning2)
	
	// Fact check - объединяем уникальные элементы
	combined["fact_check"] = mergeFactCheck(data1, data2)
	
	// Manipulations - объединяем уникальные
	combined["manipulations"] = mergeArrays(
		getArray(data1, "manipulations"),
		getArray(data2, "manipulations"),
	)
	
	// Logical issues - объединяем уникальные
	combined["logical_issues"] = mergeArrays(
		getArray(data1, "logical_issues"),
		getArray(data2, "logical_issues"),
	)
	
	// Sources - объединяем
	combined["sources"] = mergeSources(data1, data2)
	
	// Конвертируем обратно в JSON
	result, err := json.MarshalIndent(combined, "", "  ")
	if err != nil {
		log.Printf("[OPENROUTER] ⚠ Ошибка объединения, возвращаю первый ответ")
		return response1
	}
	
	log.Printf("[OPENROUTER] ✅ Результаты успешно объединены")
	log.Printf("[OPENROUTER] 📊 Итоговая оценка: %d/10 (среднее из %d и %d)", 
		(score1+score2)/2, score1, score2)
	
	return string(result)
}

func extractJSONFromResponse(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end != -1 && end > start {
		return text[start : end+1]
	}
	return text
}

func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getInt(data map[string]interface{}, key string) int {
	if val, ok := data[key]; ok {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}
	return 0
}

func getArray(data map[string]interface{}, key string) []string {
	if val, ok := data[key]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return []string{}
}

func mergeArrays(arr1, arr2 []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	
	for _, item := range arr1 {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	
	for _, item := range arr2 {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	
	return result
}

func mergeFactCheck(data1, data2 map[string]interface{}) map[string]interface{} {
	fc1, _ := data1["fact_check"].(map[string]interface{})
	fc2, _ := data2["fact_check"].(map[string]interface{})
	
	if fc1 == nil {
		fc1 = make(map[string]interface{})
	}
	if fc2 == nil {
		fc2 = make(map[string]interface{})
	}
	
	result := make(map[string]interface{})
	
	result["verifiable_facts"] = mergeArrays(
		getArray(fc1, "verifiable_facts"),
		getArray(fc2, "verifiable_facts"),
	)
	
	result["opinions_as_facts"] = mergeArrays(
		getArray(fc1, "opinions_as_facts"),
		getArray(fc2, "opinions_as_facts"),
	)
	
	result["missing_evidence"] = mergeArrays(
		getArray(fc1, "missing_evidence"),
		getArray(fc2, "missing_evidence"),
	)
	
	return result
}

func mergeSources(data1, data2 map[string]interface{}) []interface{} {
	sources1, _ := data1["sources"].([]interface{})
	sources2, _ := data2["sources"].([]interface{})
	
	seen := make(map[string]bool)
	result := []interface{}{}
	
	for _, src := range sources1 {
		if srcMap, ok := src.(map[string]interface{}); ok {
			if url, ok := srcMap["url"].(string); ok {
				if !seen[url] {
					seen[url] = true
					result = append(result, src)
				}
			}
		}
	}
	
	for _, src := range sources2 {
		if srcMap, ok := src.(map[string]interface{}); ok {
			if url, ok := srcMap["url"].(string); ok {
				if !seen[url] {
					seen[url] = true
					result = append(result, src)
				}
			}
		}
	}
	
	return result
}
