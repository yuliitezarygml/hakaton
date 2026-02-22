package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

type SerperClient struct {
	APIKey string
}

type SerperRequest struct {
	Q  string `json:"q"`
	Gl string `json:"gl,omitempty"` // Геолокация (md, ru, us, etc)
	Hl string `json:"hl,omitempty"` // Язык (ru, en, ro, etc)
	Num int   `json:"num,omitempty"` // Количество результатов
}

type SerperResponse struct {
	Organic      []SerperResult `json:"organic"`
	News         []SerperResult `json:"news"`
	KnowledgeGraph map[string]interface{} `json:"knowledgeGraph,omitempty"`
}

type SerperResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
	Date    string `json:"date,omitempty"`
}

func NewSerperClient(apiKey string) *SerperClient {
	return &SerperClient{APIKey: apiKey}
}

func (s *SerperClient) Search(query string) ([]SerperResult, error) {
	log.Printf("[SERPER] 🔍 Поиск в Google: \"%s\"", query)
	
	reqBody := SerperRequest{
		Q:   query,
		Gl:  "md", // Молдова
		Hl:  "ru", // Русский язык
		Num: 10,   // Количество результатов
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("[SERPER] ❌ Ошибка маршалинга: %v", err)
		return nil, fmt.Errorf("ошибка маршалинга: %w", err)
	}

	req, err := http.NewRequest("POST", "https://google.serper.dev/search", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("[SERPER] ❌ Ошибка создания запроса: %v", err)
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("X-API-KEY", s.APIKey)
	req.Header.Set("Content-Type", "application/json")

	log.Printf("[SERPER] 📡 Отправляю запрос к Serper API...")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[SERPER] ❌ Ошибка выполнения запроса: %v", err)
		return nil, fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[SERPER] ✓ Получен ответ: статус %d", resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[SERPER] ❌ Ошибка чтения ответа: %v", err)
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[SERPER] ❌ API вернул ошибку %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	var serperResp SerperResponse
	if err := json.Unmarshal(body, &serperResp); err != nil {
		log.Printf("[SERPER] ❌ Ошибка парсинга ответа: %v", err)
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	// Объединяем органические результаты и новости
	results := append(serperResp.Organic, serperResp.News...)
	
	log.Printf("[SERPER] ✓ Найдено результатов: %d", len(results))
	
	return results, nil
}

// SearchMultiLanguage - поиск на трех языках (русский, английский, румынский)
func (s *SerperClient) SearchMultiLanguage(query string) ([]SerperResult, error) {
	log.Printf("[SERPER] 🌍 Многоязычный поиск: \"%s\"", query)
	
	var allResults []SerperResult
	
	// Конфигурации для разных языков
	configs := []struct {
		gl   string
		hl   string
		name string
	}{
		{"md", "ru", "Русский (Молдова)"},
		{"us", "en", "English (USA)"},
		{"md", "ro", "Română (Moldova)"},
	}
	
	for _, cfg := range configs {
		log.Printf("[SERPER] 🔍 Поиск на языке: %s", cfg.name)
		
		reqBody := SerperRequest{
			Q:   query,
			Gl:  cfg.gl,
			Hl:  cfg.hl,
			Num: 5,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			log.Printf("[SERPER] ⚠ Ошибка для %s: %v", cfg.name, err)
			continue
		}

		req, err := http.NewRequest("POST", "https://google.serper.dev/search", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("[SERPER] ⚠ Ошибка создания запроса для %s: %v", cfg.name, err)
			continue
		}

		req.Header.Set("X-API-KEY", s.APIKey)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[SERPER] ⚠ Ошибка запроса для %s: %v", cfg.name, err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		
		if err != nil {
			log.Printf("[SERPER] ⚠ Ошибка чтения для %s: %v", cfg.name, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("[SERPER] ⚠ Ошибка %d для %s", resp.StatusCode, cfg.name)
			continue
		}

		var serperResp SerperResponse
		if err := json.Unmarshal(body, &serperResp); err != nil {
			log.Printf("[SERPER] ⚠ Ошибка парсинга для %s: %v", cfg.name, err)
			continue
		}

		results := append(serperResp.Organic, serperResp.News...)
		log.Printf("[SERPER] ✓ %s: найдено %d результатов", cfg.name, len(results))
		allResults = append(allResults, results...)
	}
	
	log.Printf("[SERPER] ✅ Всего найдено результатов: %d", len(allResults))
	return allResults, nil
}

func (s *SerperClient) SearchForFactCheck(text string) (string, error) {
	// Извлекаем ключевые утверждения для проверки
	keywords := extractKeywords(text)
	if len(keywords) == 0 {
		log.Printf("[SERPER] ⚠ Не удалось извлечь ключевые слова")
		return "", nil
	}

	query := strings.Join(keywords[:min(3, len(keywords))], " ")
	log.Printf("[SERPER] 🔑 Ключевые слова для поиска: %s", query)
	
	// Используем многоязычный поиск
	results, err := s.SearchMultiLanguage(query)
	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return "Результаты поиска не найдены", nil
	}

	// Форматируем результаты для AI
	var builder strings.Builder
	builder.WriteString("🌐 РЕЗУЛЬТАТЫ ПОИСКА В ИНТЕРНЕТЕ (RU/EN/RO):\n\n")
	
	count := 0
	for _, result := range results {
		if count >= 10 { // Ограничиваем 10 результатами
			break
		}
		builder.WriteString(fmt.Sprintf("%d. %s\n", count+1, result.Title))
		builder.WriteString(fmt.Sprintf("   🔗 %s\n", result.Link))
		if result.Snippet != "" {
			builder.WriteString(fmt.Sprintf("   📝 %s\n", result.Snippet))
		}
		if result.Date != "" {
			builder.WriteString(fmt.Sprintf("   📅 %s\n", result.Date))
		}
		builder.WriteString("\n")
		count++
	}

	return builder.String(), nil
}

func extractKeywords(text string) []string {
	// Простое извлечение слов (можно улучшить)
	words := strings.Fields(text)
	var keywords []string
	
	// Фильтруем короткие слова и стоп-слова (русский, английский, румынский)
	stopWords := map[string]bool{
		// Русский
		"и": true, "в": true, "на": true, "с": true, "по": true,
		"для": true, "как": true, "что": true, "это": true, "все": true,
		"или": true, "но": true, "не": true, "из": true, "от": true,
		// English
		"the": true, "is": true, "and": true, "or": true, "a": true,
		"an": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		// Română
		"și": true, "în": true, "pe": true, "cu": true, "de": true,
		"la": true, "pentru": true, "sau": true, "dar": true, "este": true,
	}
	
	for _, word := range words {
		word = strings.ToLower(strings.Trim(word, ".,!?;:\"'()[]{}"))
		if len(word) > 3 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}
	
	return keywords
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
