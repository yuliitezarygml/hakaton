package services

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type ContentFetcher struct{}

func NewContentFetcher() *ContentFetcher {
	return &ContentFetcher{}
}

func (f *ContentFetcher) FetchURL(url string) (string, error) {
	log.Printf("[FETCHER] 🌐 Начинаю загрузку контента с URL: %s", url)

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")

	log.Printf("[FETCHER] 📡 Отправляю HTTP запрос...")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ошибка загрузки: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[FETCHER] ✓ Получен ответ: статус %d", resp.StatusCode)
	log.Printf("[FETCHER] 📄 Content-Type: %s", resp.Header.Get("Content-Type"))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("статус код: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения: %w", err)
	}

	log.Printf("[FETCHER] ✓ Загружено %d байт", len(body))

	content := f.extractText(string(body))
	log.Printf("[FETCHER] ✓ Извлечено %d символов текста", len(content))
	if len(content) > 0 {
		log.Printf("[FETCHER] 📝 Первые 100 символов: %s...", truncate(content, 100))
	}

	if len(content) < 200 {
		log.Printf("[FETCHER] ⚠ Контент очень короткий (%d символов). Возможно это SPA или страница без статического текста.", len(content))
		return "", fmt.Errorf("недостаточно текстового контента на странице (%d символов). Попробуйте другую ссылку", len(content))
	}

	return content, nil
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

// ── HTML-парсер на golang.org/x/net/html ─────────────────────────────────────
//
// Преимущества перед regex:
//   - Корректно обрабатывает вложенные теги любой глубины
//   - Автоматически декодирует HTML-entities (&amp; &#x27; и т.д.)
//   - Не ломается на JSX-атрибутах с >, CDATA, комментариях
//   - Восстанавливает сломанный HTML (как браузер)

// Теги, чьё поддерево полностью пропускается
var skipTags = map[string]bool{
	"script":   true,
	"style":    true,
	"noscript": true,
	"iframe":   true,
	"svg":      true,
	"canvas":   true,
	"audio":    true,
	"video":    true,
}

// Классы/id указывающие на мусор (реклама, навигация, виджеты)
var junkAttrRe = regexp.MustCompile(`(?i)\b(ad-|ads-|advert|advertisement|banner|cookie-banner|gdpr|subscribe-|newsletter|promo|popup|modal|overlay|sponsored)\b`)

// Блочные теги, после которых нужен перенос строки
var blockTags = map[string]bool{
	"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"div": true, "section": true, "article": true, "main": true,
	"blockquote": true, "li": true, "dt": true, "dd": true,
	"tr": true, "td": true, "th": true, "br": true,
	"figcaption": true,
}

// Параграфные теги — двойной перенос (новый абзац)
var paraTags = map[string]bool{
	"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "figcaption": true,
}

func isJunkNode(n *html.Node) bool {
	// Проверяем только явные рекламные блоки
	for _, attr := range n.Attr {
		switch attr.Key {
		case "class", "id":
			val := strings.ToLower(attr.Val)
			// Только явная реклама и попапы
			if strings.Contains(val, "advertisement") || 
			   strings.Contains(val, "ad-banner") ||
			   strings.Contains(val, "popup") ||
			   strings.Contains(val, "modal") ||
			   strings.Contains(val, "cookie-banner") {
				return true
			}
		case "aria-hidden":
			if attr.Val == "true" {
				return true
			}
		}
	}
	return false
}

func (f *ContentFetcher) extractText(htmlStr string) string {
	log.Printf("[FETCHER] 🔍 Парсю HTML через golang.org/x/net/html...")

	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		log.Printf("[FETCHER] ⚠ Ошибка парсинга: %v", err)
		return ""
	}

	// Сначала пытаемся найти основной контент по семантическим тегам
	mainContent := f.findMainContent(doc)
	if mainContent != nil {
		log.Printf("[FETCHER] ✓ Найден основной контент в семантических тегах")
		return f.extractFromNode(mainContent)
	}

	// Если не нашли - парсим всю страницу
	log.Printf("[FETCHER] ⚠ Основной контент не найден, парсю всю страницу")
	return f.extractFromNode(doc)
}

// findMainContent ищет основной контент статьи по семантическим тегам
func (f *ContentFetcher) findMainContent(n *html.Node) *html.Node {
	// Приоритет 1: <article>
	if article := f.findTag(n, "article"); article != nil {
		return article
	}
	
	// Приоритет 2: <main>
	if main := f.findTag(n, "main"); main != nil {
		return main
	}
	
	// Приоритет 3: элемент с классом/id содержащим "content", "article", "post", "entry"
	if content := f.findByClass(n, []string{"content", "article", "post", "entry", "main-content", "post-content"}); content != nil {
		return content
	}
	
	return nil
}

// findTag ищет первый элемент с указанным тегом
func (f *ContentFetcher) findTag(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && strings.ToLower(n.Data) == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := f.findTag(c, tag); result != nil {
			return result
		}
	}
	return nil
}

// findByClass ищет элемент с классом/id содержащим одно из ключевых слов
func (f *ContentFetcher) findByClass(n *html.Node, keywords []string) *html.Node {
	if n.Type == html.ElementNode {
		for _, attr := range n.Attr {
			if attr.Key == "class" || attr.Key == "id" {
				val := strings.ToLower(attr.Val)
				for _, keyword := range keywords {
					if strings.Contains(val, keyword) {
						return n
					}
				}
			}
		}
	}
	
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := f.findByClass(c, keywords); result != nil {
			return result
		}
	}
	return nil
}

// extractFromNode извлекает текст из узла
func (f *ContentFetcher) extractFromNode(root *html.Node) string {
	var sb strings.Builder

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)

			// Пропускаем мусорные поддеревья целиком
			if skipTags[tag] || isJunkNode(n) {
				return
			}

			// Перед блочным элементом — перенос (если ещё нет)
			if blockTags[tag] {
				s := sb.String()
				if len(s) > 0 && s[len(s)-1] != '\n' {
					sb.WriteByte('\n')
				}
			}

			// Рекурсия в дочерние узлы
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}

			// После блочного элемента — одинарный или двойной перенос
			if blockTags[tag] {
				if paraTags[tag] {
					sb.WriteString("\n\n")
				} else {
					s := sb.String()
					if len(s) == 0 || s[len(s)-1] != '\n' {
						sb.WriteByte('\n')
					}
				}
			}
			return // дочерние уже обошли выше
		}

		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				s := sb.String()
				// Пробел между словами если предыдущий символ не перенос и не пробел
				if len(s) > 0 && s[len(s)-1] != '\n' && s[len(s)-1] != ' ' {
					sb.WriteByte(' ')
				}
				sb.WriteString(text)
			}
			return
		}

		// Для остальных типов (Document, Doctype и т.д.) — просто обходим детей
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(root)

	// ── Нормализация ──────────────────────────────────────────────
	spaceRe   := regexp.MustCompile(`[ \t]+`)
	newlineRe := regexp.MustCompile(`\n{3,}`)

	rawLines := strings.Split(sb.String(), "\n")
	var cleanLines []string
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		line = spaceRe.ReplaceAllString(line, " ")
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	text := strings.TrimSpace(newlineRe.ReplaceAllString(strings.Join(cleanLines, "\n"), "\n\n"))

	// Ограничиваем длину
	if len([]rune(text)) > 20000 {
		runes := []rune(text)
		log.Printf("[FETCHER] ⚠ Текст слишком длинный (%d симв.), обрезаю до 20000", len(runes))
		text = string(runes[:20000]) + "\n\n[...текст обрезан для анализа...]"
	}

	return text
}
