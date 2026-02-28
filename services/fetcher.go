package services

import (
	"encoding/json"
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

func isFacebookURL(u string) bool {
	return strings.Contains(u, "facebook.com/") || strings.Contains(u, "fb.com/") || strings.Contains(u, "fb.watch/")
}

// toMbasic converts a facebook.com URL to mbasic.facebook.com for lightweight HTML scraping.
func toMbasic(u string) string {
	u = strings.Replace(u, "https://www.facebook.com", "https://mbasic.facebook.com", 1)
	u = strings.Replace(u, "https://facebook.com", "https://mbasic.facebook.com", 1)
	u = strings.Replace(u, "https://m.facebook.com", "https://mbasic.facebook.com", 1)
	// Strip tracking params that can cause redirects to login
	if idx := strings.Index(u, "?"); idx != -1 {
		// Keep the base URL only — tracking params break mbasic
		u = u[:idx]
	}
	// Remove trailing # fragments
	if idx := strings.Index(u, "#"); idx != -1 {
		u = u[:idx]
	}
	return u
}

func (f *ContentFetcher) FetchURL(url string) (string, error) {
	log.Printf("[FETCHER] 🌐 Начинаю загрузку контента с URL: %s", url)

	// Facebook requires special handling via mbasic.facebook.com
	if isFacebookURL(url) {
		return f.fetchFacebook(url)
	}

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
	contentType := resp.Header.Get("Content-Type")
	log.Printf("[FETCHER] 📄 Content-Type: %s", contentType)

	// Блокируем бинарные форматы — только HTML/текст можно анализировать
	ct := strings.ToLower(contentType)
	blockedTypes := []string{
		"application/pdf",
		"application/msword",
		"application/vnd.openxmlformats",
		"application/vnd.ms-",
		"application/zip",
		"application/octet-stream",
		"image/",
		"video/",
		"audio/",
	}
	for _, blocked := range blockedTypes {
		if strings.Contains(ct, blocked) {
			ext := strings.Split(ct, "/")
			typeName := "бинарный файл"
			if strings.Contains(ct, "pdf") {
				typeName = "PDF"
			} else if strings.Contains(ct, "image") {
				typeName = "изображение"
			} else if strings.Contains(ct, "video") {
				typeName = "видео"
			} else if strings.Contains(ct, "word") || strings.Contains(ct, "office") {
				typeName = "документ Word"
			} else if len(ext) > 1 {
				typeName = ext[1]
			}
			return "", fmt.Errorf("❌ Невозможно проанализировать %s.\nПередайте ссылку на статью или веб-страницу (HTML), а не на файл", typeName)
		}
	}

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
		log.Printf("[FETCHER] ⚠ Контент очень короткий (%d символов), пробую фолбеки для SPA...", len(content))

		// Фолбек 1: ld+json (структурированные данные статьи)
		if ldContent := f.extractLdJson(string(body)); len(ldContent) >= 200 {
			log.Printf("[FETCHER] ✓ Извлечено %d символов из ld+json", len(ldContent))
			return ldContent, nil
		}

		// Фолбек 2: Open Graph + meta теги
		if metaContent := f.extractMetaTags(string(body)); len(metaContent) >= 50 {
			log.Printf("[FETCHER] ✓ Извлечено %d символов из meta-тегов", len(metaContent))
			return metaContent, nil
		}

		return "", fmt.Errorf("недостаточно текстового контента на странице (%d символов). Сайт, вероятно, использует JavaScript для рендеринга", len(content))
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
	spaceRe := regexp.MustCompile(`[ \t]+`)
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

// extractLdJson ищет structured data (JSON-LD) и вытягивает текст статьи
func (f *ContentFetcher) extractLdJson(htmlStr string) string {
	// Ищем все <script type="application/ld+json">
	re := regexp.MustCompile(`(?i)<script[^>]+type=["']application/ld\+json["'][^>]*>([\s\S]*?)</script>`)
	matches := re.FindAllStringSubmatch(htmlStr, -1)

	for _, m := range matches {
		raw := strings.TrimSpace(m[1])
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			continue
		}

		var parts []string

		if h, ok := data["headline"].(string); ok && h != "" {
			parts = append(parts, h)
		}
		if desc, ok := data["description"].(string); ok && desc != "" {
			parts = append(parts, desc)
		}
		if body, ok := data["articleBody"].(string); ok && body != "" {
			parts = append(parts, body)
		}
		if text, ok := data["text"].(string); ok && text != "" {
			parts = append(parts, text)
		}

		if len(parts) > 0 {
			result := strings.Join(parts, "\n\n")
			runes := []rune(result)
			if len(runes) > 20000 {
				result = string(runes[:20000])
			}
			return result
		}
	}
	return ""
}

// fetchFacebook fetches a public Facebook post via mbasic.facebook.com.
// mbasic serves simple HTML without JavaScript and works for public posts.
func (f *ContentFetcher) fetchFacebook(originalURL string) (string, error) {
	mbasicURL := toMbasic(originalURL)
	log.Printf("[FETCHER] 📘 Facebook → mbasic: %s", mbasicURL)

	client := &http.Client{
		Timeout: 30 * time.Second,
		// Follow redirects but stop if we land on login page
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if strings.Contains(req.URL.String(), "/login") || strings.Contains(req.URL.String(), "login.php") {
				return fmt.Errorf("Facebook требует авторизации для этого поста")
			}
			if len(via) >= 5 {
				return fmt.Errorf("слишком много перенаправлений")
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", mbasicURL, nil)
	if err != nil {
		return "", fmt.Errorf("ошибка создания запроса: %w", err)
	}

	// Mobile browser UA — mbasic works best with mobile agents
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 12; Pixel 6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.210 Mobile Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")

	resp, err := client.Do(req)
	if err != nil {
		// Give a friendly message if it's a login redirect
		if strings.Contains(err.Error(), "авторизации") {
			return "", fmt.Errorf("❌ Facebook пост закрытый или требует входа в аккаунт. Используйте публичные посты")
		}
		return "", fmt.Errorf("ошибка загрузки Facebook: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[FETCHER] ✓ mbasic ответил: %d", resp.StatusCode)

	// mbasic may return 302 to login — check final URL
	if strings.Contains(resp.Request.URL.String(), "/login") {
		return "", fmt.Errorf("❌ Facebook требует авторизации для этого поста. Используйте публичные посты")
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Facebook mbasic статус: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения: %w", err)
	}

	log.Printf("[FETCHER] ✓ Facebook: загружено %d байт", len(body))

	// Extract post text — mbasic has a simpler DOM
	content := f.extractFacebookPost(string(body))
	if len(content) < 50 {
		// Fallback: try Open Graph meta tags (og:description contains post preview)
		content = f.extractMetaTags(string(body))
		if len(content) < 50 {
			return "", fmt.Errorf("❌ Не удалось извлечь текст поста Facebook. Возможно, пост приватный или удалён")
		}
		log.Printf("[FETCHER] ✓ Facebook: использован fallback meta-теги (%d симв.)", len(content))
	} else {
		log.Printf("[FETCHER] ✓ Facebook: извлечено %d символов текста поста", len(content))
	}

	return content, nil
}

// extractFacebookPost extracts the post content from mbasic.facebook.com HTML.
// mbasic wraps post text in <div data-ft="..."> or <p> inside the story container.
func (f *ContentFetcher) extractFacebookPost(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return ""
	}

	var parts []string
	seen := map[string]bool{}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)

			// Skip nav/header/footer/script/style
			switch tag {
			case "script", "style", "nav", "footer", "head":
				return
			}

			// mbasic wraps post body in <div data-ft> or divs with id containing "story"
			isStoryDiv := false
			for _, attr := range n.Attr {
				if attr.Key == "data-ft" {
					isStoryDiv = true
				}
				if (attr.Key == "id" || attr.Key == "class") &&
					(strings.Contains(attr.Val, "story") || strings.Contains(attr.Val, "post") || strings.Contains(attr.Val, "userContent")) {
					isStoryDiv = true
				}
			}

			if isStoryDiv {
				text := strings.TrimSpace(f.extractFromNode(n))
				if len(text) > 30 && !seen[text] {
					seen[text] = true
					parts = append(parts, text)
				}
				return // don't recurse further into this node
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if len(parts) == 0 {
		// Broad fallback: just extract all visible text
		return f.extractText(htmlStr)
	}

	result := strings.Join(parts, "\n\n")
	if len([]rune(result)) > 10000 {
		runes := []rune(result)
		result = string(runes[:10000])
	}
	return result
}

// extractMetaTags вытягивает Open Graph и стандартные meta-теги
func (f *ContentFetcher) extractMetaTags(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return ""
	}

	meta := map[string]string{}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.ToLower(n.Data) == "meta" {
			var property, name, content string
			for _, attr := range n.Attr {
				switch strings.ToLower(attr.Key) {
				case "property":
					property = strings.ToLower(attr.Val)
				case "name":
					name = strings.ToLower(attr.Val)
				case "content":
					content = attr.Val
				}
			}
			key := property
			if key == "" {
				key = name
			}
			if key != "" && content != "" {
				meta[key] = content
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	var parts []string

	// Приоритет: og:title > twitter:title > title
	for _, key := range []string{"og:title", "twitter:title"} {
		if v, ok := meta[key]; ok {
			parts = append(parts, v)
			break
		}
	}
	// Описание
	for _, key := range []string{"og:description", "twitter:description", "description"} {
		if v, ok := meta[key]; ok {
			parts = append(parts, v)
			break
		}
	}
	// Доп. поля статьи
	for _, key := range []string{"article:section", "article:tag"} {
		if v, ok := meta[key]; ok {
			parts = append(parts, v)
		}
	}

	log.Printf("[FETCHER] 📋 Meta-теги: title=%q desc=%q", meta["og:title"], meta["og:description"])
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
