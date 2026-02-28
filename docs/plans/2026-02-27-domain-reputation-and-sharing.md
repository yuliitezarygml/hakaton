# Domain Reputation + Share Result Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Накапливать репутацию доменов по всем анализам и давать пользователям возможность поделиться результатом по уникальной ссылке.

**Architecture:** Две новые таблицы в Postgres (`domain_stats`, `shared_results`). После каждого URL-анализа — UPSERT в `domain_stats`. Share создаёт UUID-запись; публичная HTML-страница `/s/:id` отдаётся Go-шаблоном. Chrome extension показывает репутацию домена ещё до анализа. Telegram-бот добавляет inline-кнопку "Поделиться".

**Tech Stack:** Go 1.21, database/sql + lib/pq, html/template, Chrome Extension MV3, go-telegram-bot-api/v5

---

## Task 1: DB migrations — добавить таблицы domain_stats и shared_results

**Files:**
- Modify: `database/db.go`

**Step 1: Добавить CREATE TABLE для обеих таблиц в InitDB**

В `database/db.go` добавить после существующего CREATE TABLE для `analysis_results`:

```go
_, err = DB.Exec(`
    CREATE TABLE IF NOT EXISTS domain_stats (
        domain TEXT PRIMARY KEY,
        total_analyses INTEGER DEFAULT 0,
        sum_scores     INTEGER DEFAULT 0,
        avg_score      FLOAT   DEFAULT 0,
        last_analyzed_at TIMESTAMPTZ DEFAULT NOW()
    )
`)
if err != nil {
    log.Fatalf("❌ Ошибка создания таблицы domain_stats: %v", err)
}

_, err = DB.Exec(`
    CREATE TABLE IF NOT EXISTS shared_results (
        id         TEXT PRIMARY KEY,
        result     JSONB NOT NULL,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        expires_at TIMESTAMPTZ DEFAULT NOW() + INTERVAL '30 days'
    )
`)
if err != nil {
    log.Fatalf("❌ Ошибка создания таблицы shared_results: %v", err)
}
```

**Step 2: Проверить что сервер стартует без ошибок**

```bash
cd D:/project/openrouter-web && go build ./... && echo "OK"
```
Expected: `OK`

**Step 3: Commit**

```bash
git add database/db.go
git commit -m "feat: add domain_stats and shared_results tables"
```

---

## Task 2: Domain stats service — обновлять статистику после каждого URL-анализа

**Files:**
- Create: `services/domain.go`
- Modify: `services/analyzer.go` (метод AnalyzeURL)

**Step 1: Создать `services/domain.go`**

```go
package services

import (
	"log"
	"net/url"
	"strings"
	"text-analyzer/database"
)

// NormalizeDomain извлекает хост из URL и убирает www.
func NormalizeDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	// Убрать порт если есть
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}

// UpsertDomainStats обновляет статистику домена после анализа
func UpsertDomainStats(rawURL string, score int) {
	if database.DB == nil {
		return
	}
	domain := NormalizeDomain(rawURL)
	if domain == "" {
		return
	}
	_, err := database.DB.Exec(`
		INSERT INTO domain_stats (domain, total_analyses, sum_scores, avg_score, last_analyzed_at)
		VALUES ($1, 1, $2, $2, NOW())
		ON CONFLICT (domain) DO UPDATE SET
			total_analyses   = domain_stats.total_analyses + 1,
			sum_scores       = domain_stats.sum_scores + $2,
			avg_score        = (domain_stats.sum_scores + $2)::float / (domain_stats.total_analyses + 1),
			last_analyzed_at = NOW()
	`, domain, score)
	if err != nil {
		log.Printf("[DOMAIN] ⚠ Ошибка обновления stats для %s: %v", domain, err)
	} else {
		log.Printf("[DOMAIN] ✓ Stats обновлены: %s score=%d", domain, score)
	}
}
```

**Step 2: Вызвать UpsertDomainStats в конце AnalyzeURL (services/analyzer.go)**

Найти в `AnalyzeURL` строку `response.SourceURL = url` и добавить после неё:

```go
response.SourceURL = url
// Обновляем репутацию домена
UpsertDomainStats(url, response.CredibilityScore)
return response, nil
```

**Step 3: Собрать**

```bash
cd D:/project/openrouter-web && go build ./... && echo "OK"
```

**Step 4: Commit**

```bash
git add services/domain.go services/analyzer.go
git commit -m "feat: track domain reputation after each URL analysis"
```

---

## Task 3: Domain API endpoints

**Files:**
- Create: `handlers/domain.go`
- Modify: `main.go`

**Step 1: Создать `handlers/domain.go`**

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"text-analyzer/database"
	"text-analyzer/services"
)

type DomainHandler struct{}

func NewDomainHandler() *DomainHandler { return &DomainHandler{} }

type DomainStats struct {
	Domain          string  `json:"domain"`
	TotalAnalyses   int     `json:"total_analyses"`
	AvgScore        float64 `json:"avg_score"`
	Verdict         string  `json:"verdict"`
	LastAnalyzedAt  string  `json:"last_analyzed_at"`
}

func verdictFromScore(avg float64) string {
	switch {
	case avg >= 7:
		return "надёжный"
	case avg >= 4:
		return "сомнительный"
	default:
		return "ненадёжный"
	}
}

func corsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
}

// GetDomain — GET /api/domain/<domain>
func (h *DomainHandler) GetDomain(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	// Extract domain from path: /api/domain/example.com
	raw := strings.TrimPrefix(r.URL.Path, "/api/domain/")
	domain := services.NormalizeDomain("https://" + raw)
	if domain == "" || database.DB == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var s DomainStats
	err := database.DB.QueryRow(`
		SELECT domain, total_analyses, avg_score, last_analyzed_at
		FROM domain_stats WHERE domain = $1
	`, domain).Scan(&s.Domain, &s.TotalAnalyses, &s.AvgScore, &s.LastAnalyzedAt)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "домен не найден"})
		return
	}
	s.Verdict = verdictFromScore(s.AvgScore)
	json.NewEncoder(w).Encode(s)
}

// GetTopDomains — GET /api/domains/top
func (h *DomainHandler) GetTopDomains(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if database.DB == nil {
		json.NewEncoder(w).Encode([]DomainStats{})
		return
	}

	rows, err := database.DB.Query(`
		SELECT domain, total_analyses, avg_score, last_analyzed_at
		FROM domain_stats
		ORDER BY total_analyses DESC
		LIMIT 20
	`)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var list []DomainStats
	for rows.Next() {
		var s DomainStats
		rows.Scan(&s.Domain, &s.TotalAnalyses, &s.AvgScore, &s.LastAnalyzedAt)
		s.Verdict = verdictFromScore(s.AvgScore)
		list = append(list, s)
	}
	if list == nil {
		list = []DomainStats{}
	}
	json.NewEncoder(w).Encode(list)
}
```

**Step 2: Зарегистрировать маршруты в `main.go`**

После строки `analyzerHandler := handlers.NewAnalyzerHandler(analyzerService)` добавить:

```go
domainHandler := handlers.NewDomainHandler()
```

После строки `http.HandleFunc("/api/limits", analyzerHandler.Limits)` добавить:

```go
http.HandleFunc("/api/domain/", domainHandler.GetDomain)
http.HandleFunc("/api/domains/top", domainHandler.GetTopDomains)
```

**Step 3: Собрать и проверить**

```bash
cd D:/project/openrouter-web && go build ./... && echo "OK"
```

```bash
curl http://localhost:8080/api/domains/top
# Expected: []
```

**Step 4: Commit**

```bash
git add handlers/domain.go main.go
git commit -m "feat: add /api/domain/:domain and /api/domains/top endpoints"
```

---

## Task 4: Share endpoints (создать + получить JSON)

**Files:**
- Create: `handlers/share.go`
- Modify: `main.go`

**Step 1: Создать `handlers/share.go`**

```go
package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"text/template"
	"text-analyzer/database"
)

type ShareHandler struct {
	baseURL string
}

func NewShareHandler() *ShareHandler {
	base := os.Getenv("API_BASE")
	if base == "" {
		base = "http://localhost:8080"
	}
	return &ShareHandler{baseURL: base}
}

func newShareID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Create — POST /api/share  → {id, url}
func (h *ShareHandler) Create(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if database.DB == nil {
		http.Error(w, `{"error":"db unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	// Принимаем любой JSON (результат анализа)
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	id := newShareID()
	_, err := database.DB.Exec(
		`INSERT INTO shared_results (id, result) VALUES ($1, $2)`,
		id, []byte(raw),
	)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	shareURL := h.baseURL + "/s/" + id
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "url": shareURL})
}

// GetResult — GET /api/share/:id  → JSON result
func (h *ShareHandler) GetResult(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	id := strings.TrimPrefix(r.URL.Path, "/api/share/")
	if id == "" || database.DB == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var raw []byte
	err := database.DB.QueryRow(
		`SELECT result FROM shared_results WHERE id = $1 AND expires_at > NOW()`,
		id,
	).Scan(&raw)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "не найдено или истёк срок"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

// ShowPage — GET /s/:id  → HTML страница с результатом
var shareTmpl = template.Must(template.New("share").Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Результат проверки</title>
<style>
  *{box-sizing:border-box;margin:0;padding:0}
  body{background:#0a0e1a;color:#e2e8f0;font-family:'Segoe UI',system-ui,sans-serif;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:20px}
  .card{background:#111827;border:1px solid #1f2937;border-radius:16px;max-width:680px;width:100%;padding:32px;box-shadow:0 25px 50px rgba(0,0,0,.5)}
  .score{font-size:64px;font-weight:800;line-height:1}
  .score.green{color:#22c55e}.score.yellow{color:#eab308}.score.red{color:#ef4444}
  .verdict{font-size:20px;font-weight:600;margin:8px 0 24px;color:#94a3b8}
  .summary{color:#cbd5e1;line-height:1.6;margin-bottom:24px}
  .section{margin-bottom:20px}
  .section h3{font-size:13px;text-transform:uppercase;letter-spacing:.1em;color:#64748b;margin-bottom:10px}
  .tag{display:inline-block;background:#1e293b;border:1px solid #334155;border-radius:6px;padding:4px 10px;font-size:13px;margin:3px;color:#94a3b8}
  .footer{margin-top:28px;padding-top:20px;border-top:1px solid #1f2937;display:flex;align-items:center;justify-content:space-between;flex-wrap:gap}
  .footer a{color:#3b82f6;text-decoration:none;font-size:14px}
  .footer a:hover{text-decoration:underline}
  .badge{font-size:12px;color:#475569}
</style>
</head>
<body>
<div class="card" id="app">
  <div class="badge">Проверено Text Analyzer</div>
  <div id="content" style="margin-top:16px">
    <div style="color:#475569">Загрузка...</div>
  </div>
</div>
<script>
const id = location.pathname.split('/').pop();
fetch('/api/share/' + id)
  .then(r => r.json())
  .then(d => {
    const score = d.credibility_score || 0;
    const cls = score >= 7 ? 'green' : score >= 4 ? 'yellow' : 'red';
    const verdict = d.final_verdict || (score >= 7 ? '✅ Достоверно' : score >= 4 ? '🟡 Сомнительно' : '🔴 Дезинформация');
    const manips = (d.manipulations || []).map(m => '<span class="tag">'+m+'</span>').join('');
    const issues = (d.logical_issues || []).map(i => '<span class="tag">'+i+'</span>').join('');
    document.getElementById('content').innerHTML =
      '<div class="score '+cls+'">'+score+'/10</div>' +
      '<div class="verdict">'+verdict+'</div>' +
      '<div class="summary">'+( d.summary || '')+'</div>' +
      (manips ? '<div class="section"><h3>Манипуляции</h3>'+manips+'</div>' : '') +
      (issues ? '<div class="section"><h3>Логические ошибки</h3>'+issues+'</div>' : '') +
      '<div class="footer">' +
        '<a href="/">Проверить свою статью →</a>' +
        '<span class="badge">Поделились результатом</span>' +
      '</div>';
  })
  .catch(() => {
    document.getElementById('content').innerHTML = '<div style="color:#ef4444">Ссылка устарела или не существует</div>';
  });
</script>
</body>
</html>`))

func (h *ShareHandler) ShowPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	shareTmpl.Execute(w, nil)
}
```

**Step 2: Зарегистрировать в `main.go`**

После `domainHandler := handlers.NewDomainHandler()` добавить:

```go
shareHandler := handlers.NewShareHandler()
```

После `http.HandleFunc("/api/domains/top", domainHandler.GetTopDomains)` добавить:

```go
http.HandleFunc("/api/share", shareHandler.Create)
http.HandleFunc("/api/share/", shareHandler.GetResult)
http.HandleFunc("/s/", shareHandler.ShowPage)
```

**Step 3: Собрать**

```bash
cd D:/project/openrouter-web && go build ./... && echo "OK"
```

**Step 4: Smoke-test (опционально, нужна запущенная БД)**

```bash
# Создать шаренный результат
curl -s -X POST http://localhost:8080/api/share \
  -H "Content-Type: application/json" \
  -d '{"credibility_score":3,"summary":"тест","manipulations":["давление"]}'
# Expected: {"id":"abcd1234","url":"http://localhost:8080/s/abcd1234"}

# Открыть в браузере: http://localhost:8080/s/abcd1234
```

**Step 5: Commit**

```bash
git add handlers/share.go main.go
git commit -m "feat: add /api/share and /s/:id public share page"
```

---

## Task 5: Chrome extension — показывать репутацию домена

**Files:**
- Modify: `EXTENSION/popup.js`
- Modify: `EXTENSION/popup.html` (добавить элемент под URL)
- Modify: `EXTENSION/popup.css` (стиль для плашки репутации)

**Step 1: Добавить в `popup.html` элемент репутации**

Найти секцию `view-idle` или начало `view-result` и добавить прямо перед кнопкой "Сканировать":

```html
<!-- Вставить внутрь view-idle, перед кнопкой scan -->
<div id="domain-rep" class="domain-rep hidden"></div>
```

**Step 2: Добавить стиль в `popup.css`**

```css
.domain-rep {
  margin: 8px 0;
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: rgba(255,255,255,0.04);
  border: 1px solid rgba(255,255,255,0.08);
}
.domain-rep.rep-good  { border-color: rgba(34,197,94,.3);  color: #86efac; }
.domain-rep.rep-mid   { border-color: rgba(234,179,8,.3);  color: #fde68a; }
.domain-rep.rep-bad   { border-color: rgba(239,68,68,.3);  color: #fca5a5; }
.domain-rep .rep-score { font-weight: 700; font-size: 15px; }
.domain-rep .rep-info  { color: #94a3b8; font-size: 11px; }
```

**Step 3: Добавить функцию в `popup.js`**

В начало файла (после констант) добавить функцию `loadDomainRep`:

```js
async function loadDomainRep(url) {
  const el = document.getElementById('domain-rep');
  if (!el || !url) return;
  try {
    const u = new URL(url);
    let domain = u.hostname.replace(/^www\./, '');
    const API = 'https://apich.sinkdev.dev'; // или константа из проекта
    const res = await fetch(`${API}/api/domain/${domain}`);
    if (!res.ok) return;
    const d = await res.json();
    const avg = d.avg_score.toFixed(1);
    const cls = d.avg_score >= 7 ? 'rep-good' : d.avg_score >= 4 ? 'rep-mid' : 'rep-bad';
    const emoji = d.avg_score >= 7 ? '🟢' : d.avg_score >= 4 ? '🟡' : '🔴';
    el.className = `domain-rep ${cls}`;
    el.innerHTML = `
      <span class="rep-score">${emoji} ${avg}/10</span>
      <span>
        <b>${domain}</b><br>
        <span class="rep-info">${d.total_analyses} проверок · ${d.verdict}</span>
      </span>`;
  } catch {}
}
```

**Step 4: Вызвать при инициализации popup**

В функции `init()` (или при получении активной вкладки), после определения `currentUrl`:

```js
loadDomainRep(currentUrl);
```

**Step 5: Проверить в браузере**

Открыть popup на любом ранее анализированном сайте → должна появиться цветная плашка с репутацией.

**Step 6: Commit**

```bash
git add EXTENSION/popup.js EXTENSION/popup.html EXTENSION/popup.css
git commit -m "feat: show domain reputation in extension popup"
```

---

## Task 6: Telegram bot — кнопка "Поделиться"

**Files:**
- Modify: `telegram-bot/main.go`

**Step 1: Добавить функцию shareResult в main.go**

```go
// shareResult отправляет результат на /api/share и возвращает публичный URL
func shareResult(result string) (string, error) {
	resp, err := http.Post(apiBase+"/api/share", "application/json", strings.NewReader(result))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.URL, nil
}
```

Нужно добавить import `"encoding/json"` если его нет.

**Step 2: После отправки результата — добавить inline кнопку**

В `runAnalysis`, в блоке `case finalResult != nil:` заменить:

```go
case finalResult != nil:
    edit(chatID, msgID, FormatResult(finalResult))
```

на:

```go
case finalResult != nil:
    text := FormatResult(finalResult)
    // Попытаться создать share-ссылку
    if raw, err := json.Marshal(finalResult); err == nil {
        if shareURL, err := shareResult(string(raw)); err == nil {
            // Отправить новым сообщением с кнопкой
            edit(chatID, msgID, text)
            msg := tgbotapi.NewMessage(chatID, "")
            msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
                tgbotapi.NewInlineKeyboardRow(
                    tgbotapi.NewInlineKeyboardButtonURL("🔗 Поделиться результатом", shareURL),
                ),
            )
            bot.Send(msg)
            return
        }
    }
    edit(chatID, msgID, text)
```

**Step 3: Собрать бота**

```bash
cd D:/project/openrouter-web/telegram-bot && go build ./... && echo "OK"
```

**Step 4: Commit**

```bash
git add telegram-bot/main.go
git commit -m "feat: add share button in telegram bot after analysis"
```

---

## Task 7: Финальная проверка

**Step 1: Полный билд проекта**

```bash
cd D:/project/openrouter-web && go build ./... && echo "Backend OK"
cd telegram-bot && go build ./... && echo "Bot OK"
```

**Step 2: Проверить эндпоинты**

```bash
# Топ доменов
curl http://localhost:8080/api/domains/top

# Конкретный домен (после того как был проанализирован URL с этого домена)
curl http://localhost:8080/api/domain/example.com

# Создать share
curl -s -X POST http://localhost:8080/api/share \
  -H "Content-Type: application/json" \
  -d '{"credibility_score":7,"summary":"тестовая статья","manipulations":[]}'
```

**Step 3: Обновить tasks.md**

Добавить в `tasks.md` в раздел "Выполнено":
- `[x] Репутация доменов — накапливает avg_score по всем URL-анализам, /api/domain/:domain, /api/domains/top`
- `[x] Шеринг результата — /api/share + публичная страница /s/:id, кнопка в Telegram-боте`
- `[x] Chrome extension — плашка репутации домена в popup до анализа`
