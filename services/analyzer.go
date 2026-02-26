package services

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"text-analyzer/models"
)

// AIClient — интерфейс для любого AI провайдера (OpenRouter, Groq)
type AIClient interface {
	Analyze(text string) (string, *models.TokenUsage, error)
}

type AnalyzerService struct {
	client       AIClient
	fetcher      *ContentFetcher
	serper       *SerperClient
	promptConfig *PromptConfig
}

func NewAnalyzerService(client AIClient, fetcher *ContentFetcher, serper *SerperClient, promptConfig *PromptConfig) *AnalyzerService {
	return &AnalyzerService{
		client:       client,
		fetcher:      fetcher,
		serper:       serper,
		promptConfig: promptConfig,
	}
}

// NewAnalyzerServiceGroq — алиас для удобства (тот же конструктор)
func NewAnalyzerServiceGroq(client *GroqClient, fetcher *ContentFetcher, serper *SerperClient, promptConfig *PromptConfig) *AnalyzerService {
	return NewAnalyzerService(client, fetcher, serper, promptConfig)
}

func (s *AnalyzerService) AnalyzeText(text string, progress ...func(string)) (*models.AnalysisResponse, error) {
	report := func(msg string) {
		log.Printf("[ANALYZER] %s", msg)
		if len(progress) > 0 && progress[0] != nil {
			progress[0](msg)
		}
	}
	report(fmt.Sprintf("📄 Читаю текст... %d символов", len(text)))

	var searchContext string
	if s.serper != nil && s.serper.APIKey != "" {
		report("🔍 Ищу факты по теме в интернете...")
		searchResults, err := s.serper.SearchForFactCheck(text)
		if err != nil {
			report("⚠ Поиск в сети недоступен, продолжаю без него")
		} else if searchResults != "" {
			searchContext = "\n\n--- ИНФОРМАЦИЯ ИЗ ИНТЕРНЕТА ДЛЯ ПРОВЕРКИ ФАКТОВ ---\n" + searchResults
			report("✓ Нашёл дополнительный контекст из сети")
		} else {
			report("⚠ По теме ничего не нашлось, продолжаю без контекста")
		}
	}

	report(fmt.Sprintf("🧠 Анализирую текст на манипуляции и дезинформацию... (%d симв.)", len(text)+len(searchContext)))
	report("⏳ Проверяю источники, логику и факты...")

	fullText := text + searchContext
	rawResponse, tokenUsage, err := s.client.Analyze(fullText)
	if err != nil {
		report(fmt.Sprintf("❌ Ошибка при анализе: %v", err))
		return nil, err
	}

	report("📊 Обрабатываю результат...")
	if tokenUsage != nil {
		report(fmt.Sprintf("📊 Использовано токенов: %d (запрос: %d, ответ: %d)", 
			tokenUsage.TotalTokens, tokenUsage.PromptTokens, tokenUsage.CompletionTokens))
	}

	jsonStr := extractJSON(rawResponse)
	jsonStr = fixJSONTypes(jsonStr)

	var response models.AnalysisResponse
	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		cleanJSON := strings.ReplaceAll(jsonStr, "\n", " ")
		cleanJSON = strings.ReplaceAll(cleanJSON, "\t", " ")
		if err := json.Unmarshal([]byte(cleanJSON), &response); err != nil {
			report("❌ Не удалось обработать результат")
			return &models.AnalysisResponse{
				Summary:     "Не удалось распарсить ответ",
				RawResponse: rawResponse,
			}, nil
		}
	}

	response.RawResponse = rawResponse
	response.Usage = tokenUsage

	report(fmt.Sprintf("📊 Достоверность: %d/10 · манипуляций: %d · логических ошибок: %d",
		response.CredibilityScore, len(response.Manipulations), len(response.LogicalIssues)))
	if response.CredibilityScore <= 3 {
		report("🔴 Высокая вероятность дезинформации")
	} else if response.CredibilityScore <= 6 {
		report("🟡 Контент вызывает сомнения")
	} else {
		report("🟢 Контент выглядит достоверно")
	}

	if response.CredibilityScore <= 7 && s.serper != nil && s.serper.APIKey != "" {
		report("🔎 Проверяю по независимым источникам...")
		verification, err := s.verifyAndFindTruth(text, &response)
		if err != nil {
			report("⚠ Не удалось провести перекрёстную проверку")
		} else {
			response.Verification = *verification
			if verification.IsFake {
				report(fmt.Sprintf("🚨 Обнаружены признаки дезинформации (%d)", len(verification.FakeReasons)))
			} else {
				report("✓ Перекрёстная проверка завершена")
			}
		}
	}

	report("✅ Готово!")
	return &response, nil
}

func (s *AnalyzerService) AnalyzeURL(url string, progress ...func(string)) (*models.AnalysisResponse, error) {
	report := func(msg string) {
		log.Printf("[ANALYZER] %s", msg)
		if len(progress) > 0 && progress[0] != nil {
			progress[0](msg)
		}
	}

	report("🌐 Загружаю страницу...")

	content, err := s.fetcher.FetchURL(url)
	if err != nil {
		report(fmt.Sprintf("❌ Не удалось загрузить страницу: %v", err))
		return nil, err
	}

	report(fmt.Sprintf("✓ Страница загружена, читаю контент... (%d символов)", len(content)))
	report("🔬 Начинаю анализ содержимого...")

	var progressFn func(string)
	if len(progress) > 0 {
		progressFn = progress[0]
	}
	response, err := s.AnalyzeText(content, progressFn)
	if err != nil {
		return nil, err
	}

	response.SourceURL = url
	return response, nil
}

func extractJSON(text string) string {
	// Ищем JSON между ```json и ``` или просто { и }
	
	// Сначала пробуем найти в markdown блоке
	if strings.Contains(text, "```json") {
		start := strings.Index(text, "```json")
		if start != -1 {
			start += 7 // длина "```json"
			end := strings.Index(text[start:], "```")
			if end != -1 {
				return strings.TrimSpace(text[start : start+end])
			}
		}
	}
	
	// Ищем первый { и последний }
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	
	if start != -1 && end != -1 && end > start {
		jsonStr := text[start : end+1]
		
		// Очищаем от escape-последовательностей
		jsonStr = strings.ReplaceAll(jsonStr, "\\n", " ")
		jsonStr = strings.ReplaceAll(jsonStr, "\\t", " ")
		jsonStr = strings.ReplaceAll(jsonStr, "\\\"", "\"")
		
		// Проверяем, что это валидный JSON
		var testMap map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &testMap); err == nil {
			return jsonStr
		}
		
		log.Printf("[PARSER] ⚠ JSON невалидный, возвращаю как есть")
		return jsonStr
	}
	
	return text
}


// verifyAndFindTruth - проверяет статью и ищет настоящую информацию
func (s *AnalyzerService) verifyAndFindTruth(text string, analysis *models.AnalysisResponse) (*models.Verification, error) {
	log.Printf("[VERIFIER] 🔍 Начинаю глубокую верификацию...")
	
	verification := &models.Verification{
		IsFake:      analysis.CredibilityScore <= 5,
		FakeReasons: []string{},
	}
	
	// Собираем причины почему статья фальшивая
	if len(analysis.Manipulations) > 0 {
		verification.FakeReasons = append(verification.FakeReasons, 
			fmt.Sprintf("Найдено %d манипуляций и приемов демагогии", len(analysis.Manipulations)))
	}
	
	if len(analysis.LogicalIssues) > 0 {
		verification.FakeReasons = append(verification.FakeReasons,
			fmt.Sprintf("Обнаружено %d логических противоречий", len(analysis.LogicalIssues)))
	}
	
	if len(analysis.FactCheck.MissingEvidence) > 0 {
		verification.FakeReasons = append(verification.FakeReasons,
			fmt.Sprintf("Отсутствуют доказательства для %d утверждений", len(analysis.FactCheck.MissingEvidence)))
	}
	
	if len(analysis.FactCheck.OpinionsAsFacts) > 0 {
		verification.FakeReasons = append(verification.FakeReasons,
			fmt.Sprintf("Мнения выдаются за факты: %d случаев", len(analysis.FactCheck.OpinionsAsFacts)))
	}
	
	// Извлекаем ключевые утверждения для проверки
	keywords := extractMainClaims(text, analysis)
	if len(keywords) == 0 {
		log.Printf("[VERIFIER] ⚠ Не удалось извлечь ключевые утверждения")
		return verification, nil
	}
	
	log.Printf("[VERIFIER] 🔑 Ключевые утверждения для проверки: %v", keywords)
	
	// Ищем настоящую информацию в интернете
	var allResults []string
	var verifiedSources []models.Source
	
	for i, claim := range keywords {
		if i >= 3 { // Ограничиваем 3 запросами
			break
		}
		
		log.Printf("[VERIFIER] 🌐 Проверяю утверждение %d: %s", i+1, claim)
		
		results, err := s.serper.SearchMultiLanguage(claim)
		if err != nil {
			log.Printf("[VERIFIER] ⚠ Ошибка поиска: %v", err)
			continue
		}
		
		if len(results) > 0 {
			log.Printf("[VERIFIER] ✓ Найдено %d результатов", len(results))
			
			// Берем топ-3 результата
			for j, result := range results {
				if j >= 3 {
					break
				}
				
				allResults = append(allResults, fmt.Sprintf(
					"• %s\n  Источник: %s\n  %s",
					result.Title, result.Link, result.Snippet,
				))
				
				verifiedSources = append(verifiedSources, models.Source{
					Title:       result.Title,
					URL:         result.Link,
					Description: result.Snippet,
				})
			}
		}
	}
	
	if len(allResults) > 0 {
		verification.RealInformation = "НАСТОЯЩАЯ ИНФОРМАЦИЯ ИЗ ПРОВЕРЕННЫХ ИСТОЧНИКОВ:\n\n" + 
			strings.Join(allResults, "\n\n")
		verification.VerifiedSources = verifiedSources
		
		log.Printf("[VERIFIER] ✅ Найдена настоящая информация из %d источников", len(verifiedSources))
	} else {
		verification.RealInformation = "Не удалось найти достоверную информацию для проверки утверждений из статьи."
		log.Printf("[VERIFIER] ⚠ Настоящая информация не найдена")
	}
	
	return verification, nil
}

// extractMainClaims - извлекает основные утверждения из анализа AI (приоритет над сырым текстом)
func extractMainClaims(text string, analysis *models.AnalysisResponse) []string {
	claims := []string{}
	seen := make(map[string]bool)

	addUnique := func(s string) {
		s = strings.TrimSpace(s)
		if len(s) > 15 && len(s) < 250 && !seen[s] {
			seen[s] = true
			claims = append(claims, s)
		}
	}

	// Приоритет 1: факты из missing_evidence — это самые сомнительные утверждения
	for _, fact := range analysis.FactCheck.MissingEvidence {
		addUnique(fact)
		if len(claims) >= 2 {
			break
		}
	}

	// Приоритет 2: проверяемые факты из AI-анализа
	for _, fact := range analysis.FactCheck.VerifiableFacts {
		addUnique(fact)
		if len(claims) >= 4 {
			break
		}
	}

	// Приоритет 3: мнения, поданные как факты
	for _, op := range analysis.FactCheck.OpinionsAsFacts {
		addUnique(op)
		if len(claims) >= 5 {
			break
		}
	}

	// Fallback: если AI не дал фактов, ищем предложения с числами/именами в тексте
	if len(claims) == 0 {
		reNumbers := regexp.MustCompile(`\d`)
		sentences := strings.Split(text, ".")
		for _, sentence := range sentences {
			sentence = strings.TrimSpace(sentence)
			// Предпочитаем предложения с цифрами или заглавными словами (имена, организации)
			if len(sentence) > 30 && len(sentence) < 200 && reNumbers.MatchString(sentence) {
				addUnique(sentence)
				if len(claims) >= 3 {
					break
				}
			}
		}
		// Если с цифрами не нашли — берём первые нормальные предложения
		if len(claims) == 0 {
			for _, sentence := range sentences {
				sentence = strings.TrimSpace(sentence)
				if len(sentence) > 30 && len(sentence) < 200 {
					addUnique(sentence)
					if len(claims) >= 3 {
						break
					}
				}
			}
		}
	}

	return claims
}


// fixJSONTypes исправляет типы данных в JSON (строки -> числа/bool)
func fixJSONTypes(jsonStr string) string {
	// Исправляем credibility_score: "1" -> 1 или просто число без кавычек
	re := regexp.MustCompile(`"credibility_score"\s*:\s*"?(\d+)"?`)
	jsonStr = re.ReplaceAllString(jsonStr, `"credibility_score": $1`)
	
	// Исправляем is_fake: "true" -> true, "false" -> false
	jsonStr = strings.ReplaceAll(jsonStr, `"is_fake": "true"`, `"is_fake": true`)
	jsonStr = strings.ReplaceAll(jsonStr, `"is_fake": "false"`, `"is_fake": false`)
	jsonStr = strings.ReplaceAll(jsonStr, `"is_fake": true`, `"is_fake": true`) // уже правильно
	jsonStr = strings.ReplaceAll(jsonStr, `"is_fake": false`, `"is_fake": false`) // уже правильно
	
	return jsonStr
}
