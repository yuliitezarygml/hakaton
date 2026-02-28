package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"
)

// Distortion описывает одно конкретное искажение относительно оригинала.
type Distortion struct {
	Type     string `json:"type"`     // exaggeration | omission | addition | change
	Original string `json:"original"` // что было в оригинале
	Changed  string `json:"changed"`  // как стало в производной статье
}

// ChainNode — один узел цепочки (исходная статья или пересказ).
type ChainNode struct {
	URL              string       `json:"url"`
	Title            string       `json:"title"`
	Domain           string       `json:"domain"`
	PublishedHint    string       `json:"published_hint"` // дата если видна в тексте
	IsOriginal       bool         `json:"is_original"`
	CredibilityScore int          `json:"credibility_score"`
	KeyClaims        []string     `json:"key_claims"`
	Distortions      []Distortion `json:"distortions"`
	DistortionScore  int          `json:"distortion_score"` // 0=без искажений, 10=полностью перевран
	Summary          string       `json:"summary"`
}

// ChainResult — итоговое дерево цепочки.
type ChainResult struct {
	Topic       string      `json:"topic"`
	OriginalURL string      `json:"original_url"`
	Nodes       []ChainNode `json:"nodes"`
}

// ChainEvent передаётся клиенту по SSE по мере обработки.
type ChainEvent struct {
	Type    string       `json:"type"`
	Message string       `json:"message,omitempty"`
	Node    *ChainNode   `json:"node,omitempty"`
	Result  *ChainResult `json:"result,omitempty"`
}

// ChainService строит цепочку источников для заданного URL.
type ChainService struct {
	client  AIClient
	fetcher *ContentFetcher
	serper  *SerperClient
}

func NewChainService(client AIClient, fetcher *ContentFetcher, serper *SerperClient) *ChainService {
	return &ChainService{client: client, fetcher: fetcher, serper: serper}
}

// BuildChain — основной метод. Стримит ChainEvent через emit по мере работы.
func (s *ChainService) BuildChain(ctx context.Context, inputURL string, emit func(ChainEvent)) error {
	emit(ChainEvent{Type: "chain_start", Message: "🔍 Загружаю исходную статью..."})

	// 1. Загружаем оригинал
	content, err := s.fetcher.FetchURL(inputURL)
	if err != nil {
		return fmt.Errorf("не удалось загрузить статью: %w", err)
	}
	if len(content) < 100 {
		return fmt.Errorf("недостаточно текста для анализа")
	}
	if len([]rune(content)) > 4000 {
		runes := []rune(content)
		content = string(runes[:4000])
	}

	emit(ChainEvent{Type: "chain_progress", Message: fmt.Sprintf("✓ Загружено %d симв., извлекаю тему и утверждения...", len(content))})

	// 2. Извлекаем тему, поисковый запрос и ключевые утверждения оригинала
	topic, searchQuery, originalClaims, err := s.extractTopicAndClaims(content)
	if err != nil {
		return fmt.Errorf("ошибка извлечения темы: %w", err)
	}

	emit(ChainEvent{Type: "chain_progress", Message: fmt.Sprintf("✓ Тема: «%s» · ищу похожие публикации...", chainTruncate(topic, 60))})

	// Узел оригинала
	originalNode := ChainNode{
		URL:              inputURL,
		Title:            topic,
		Domain:           chainExtractDomain(inputURL),
		IsOriginal:       true,
		CredibilityScore: 8,
		KeyClaims:        originalClaims,
		Distortions:      []Distortion{},
		DistortionScore:  0,
		Summary:          "Исходная статья — точка отсчёта",
	}
	emit(ChainEvent{Type: "chain_node", Node: &originalNode})

	if s.serper == nil || s.serper.APIKey == "" {
		return fmt.Errorf("Serper API недоступен — поиск похожих статей невозможен")
	}

	// 3. Ищем похожие статьи через Serper
	emit(ChainEvent{Type: "chain_progress", Message: fmt.Sprintf("🌐 Поиск: «%s»...", searchQuery)})

	results, err := s.serper.SearchMultiLanguage(searchQuery)
	if err != nil {
		return fmt.Errorf("ошибка поиска: %w", err)
	}

	emit(ChainEvent{Type: "chain_progress", Message: fmt.Sprintf("✓ Найдено %d ссылок · анализирую по очереди...", len(results))})

	// 4. Анализируем каждую найденную статью
	nodes := []ChainNode{originalNode}
	analyzed := 0
	const maxNodes = 5
	originalDomain := chainExtractDomain(inputURL)

	for _, result := range results {
		if ctx.Err() != nil {
			break
		}
		if analyzed >= maxNodes {
			break
		}
		if chainExtractDomain(result.Link) == originalDomain {
			continue
		}

		emit(ChainEvent{
			Type:    "chain_progress",
			Message: fmt.Sprintf("🔎 Проверяю %s...", chainExtractDomain(result.Link)),
		})

		node, err := s.analyzeRelatedArticle(ctx, result.Link, result.Title, originalClaims)
		if err != nil {
			log.Printf("[CHAIN] ⚠ %s: %v", result.Link, err)
			continue
		}
		if !node.IsSameStory {
			log.Printf("[CHAIN] ↷ %s — другая тема, пропускаю", result.Link)
			continue
		}

		chainNode := ChainNode{
			URL:              result.Link,
			Title:            node.Title,
			Domain:           chainExtractDomain(result.Link),
			IsOriginal:       false,
			CredibilityScore: node.CredibilityScore,
			KeyClaims:        node.KeyClaims,
			Distortions:      node.Distortions,
			DistortionScore:  node.DistortionScore,
			Summary:          node.Summary,
			PublishedHint:    node.PublishedHint,
		}
		nodes = append(nodes, chainNode)
		analyzed++
		emit(ChainEvent{Type: "chain_node", Node: &chainNode})
	}

	emit(ChainEvent{
		Type:    "chain_done",
		Message: fmt.Sprintf("✅ Цепочка построена · %d источников проанализировано", len(nodes)),
		Result: &ChainResult{
			Topic:       topic,
			OriginalURL: inputURL,
			Nodes:       nodes,
		},
	})
	return nil
}

// articleAnalysis — внутренний результат парсинга ответа AI на запрос сравнения.
type articleAnalysis struct {
	IsSameStory      bool         `json:"is_same_story"`
	Title            string       `json:"title"`
	PublishedHint    string       `json:"published_hint"`
	KeyClaims        []string     `json:"key_claims"`
	Distortions      []Distortion `json:"distortions"`
	DistortionScore  int          `json:"distortion_score"`
	CredibilityScore int          `json:"credibility_score"`
	Summary          string       `json:"summary"`
}

// analyzeRelatedArticle загружает статью и сравнивает с оригиналом через AI.
func (s *ChainService) analyzeRelatedArticle(ctx context.Context, articleURL, title string, originalClaims []string) (*articleAnalysis, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_ = fetchCtx

	content, err := s.fetcher.FetchURL(articleURL)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	if len(content) < 80 {
		return nil, fmt.Errorf("слишком мало текста")
	}
	if len([]rune(content)) > 3000 {
		runes := []rune(content)
		content = string(runes[:3000])
	}

	claimsStr := "- " + strings.Join(originalClaims, "\n- ")
	if len(originalClaims) == 0 {
		claimsStr = "(ключевые утверждения не извлечены)"
	}

	prompt := fmt.Sprintf(`Ты — детектор искажений в журналистике. Сравни два материала.

ОРИГИНАЛ — ключевые утверждения:
%s

ПРОИЗВОДНЫЙ МАТЕРИАЛ:
%s

Верни ТОЛЬКО JSON без markdown:
{
  "is_same_story": true,
  "title": "заголовок производного материала",
  "published_hint": "дата если видна в тексте, иначе пустая строка",
  "key_claims": ["утверждение 1", "утверждение 2"],
  "distortions": [
    {"type": "exaggeration", "original": "что было в оригинале", "changed": "как стало"}
  ],
  "distortion_score": 3,
  "credibility_score": 7,
  "summary": "1-2 предложения об отличиях от оригинала"
}

Типы искажений:
- exaggeration: числа, масштаб или серьёзность преувеличены
- omission: важный факт или контекст намеренно опущен
- addition: добавлено непроверенное утверждение
- change: факт (число, имя, место) изменён на другой

distortion_score: 0=без искажений, 10=смысл полностью искажён.
Если статьи на разные темы — is_same_story: false, остальные поля пустые.`,
		claimsStr, content)

	rawResponse, _, err := s.client.Analyze(prompt)
	if err != nil {
		return nil, fmt.Errorf("AI: %w", err)
	}

	jsonStr := extractJSON(rawResponse)
	jsonStr = fixJSONTypes(jsonStr)

	var analysis articleAnalysis
	if err := json.Unmarshal([]byte(jsonStr), &analysis); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if analysis.Title == "" {
		analysis.Title = title
	}
	if analysis.Distortions == nil {
		analysis.Distortions = []Distortion{}
	}
	return &analysis, nil
}

// extractTopicAndClaims извлекает тему, поисковый запрос и ключевые утверждения.
func (s *ChainService) extractTopicAndClaims(text string) (topic, searchQuery string, claims []string, err error) {
	prompt := fmt.Sprintf(`Проанализируй статью и верни ТОЛЬКО JSON без markdown:

%s

Ответ:
{
  "topic": "краткая тема статьи (до 70 символов)",
  "search_query": "поисковый запрос Google для поиска похожих статей (5-8 слов на языке статьи)",
  "key_claims": ["главное утверждение 1", "главное утверждение 2", "главное утверждение 3"]
}`, text)

	rawResponse, _, err := s.client.Analyze(prompt)
	if err != nil {
		return "", "", nil, err
	}

	jsonStr := extractJSON(rawResponse)
	var result struct {
		Topic       string   `json:"topic"`
		SearchQuery string   `json:"search_query"`
		KeyClaims   []string `json:"key_claims"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return "", "", nil, fmt.Errorf("parse topic: %w", err)
	}
	if result.Topic == "" {
		result.Topic = "Анализируемая статья"
	}
	if result.SearchQuery == "" {
		result.SearchQuery = result.Topic
	}
	return result.Topic, result.SearchQuery, result.KeyClaims, nil
}

// chainExtractDomain извлекает hostname из URL без www.
func chainExtractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

// chainTruncate обрезает строку до max рун, добавляя "…".
func chainTruncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
