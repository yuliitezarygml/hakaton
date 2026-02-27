package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"strings"
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

// Create — POST /api/share → {"id":"…","url":"…"}
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

// GetResult — GET /api/share/:id → raw JSON result
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "не найдено или истёк срок"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

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
.footer{margin-top:28px;padding-top:20px;border-top:1px solid #1f2937;display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:8px}
.footer a{color:#3b82f6;text-decoration:none;font-size:14px}
.footer a:hover{text-decoration:underline}
.badge{font-size:12px;color:#475569}
</style>
</head>
<body>
<div class="card">
  <div class="badge">Проверено Text Analyzer</div>
  <div id="content" style="margin-top:16px;color:#475569">Загрузка...</div>
</div>
<script>
const id=location.pathname.split('/').pop();
fetch('/api/share/'+id)
  .then(r=>r.json())
  .then(d=>{
    const s=d.credibility_score||0;
    const cls=s>=7?'green':s>=4?'yellow':'red';
    const v=d.final_verdict||(s>=7?'✅ Достоверно':s>=4?'🟡 Сомнительно':'🔴 Дезинформация');
    const manips=(d.manipulations||[]).map(m=>'<span class="tag">'+m+'</span>').join('');
    const issues=(d.logical_issues||[]).map(i=>'<span class="tag">'+i+'</span>').join('');
    document.getElementById('content').innerHTML=
      '<div class="score '+cls+'">'+s+'/10</div>'+
      '<div class="verdict">'+v+'</div>'+
      '<div class="summary">'+(d.summary||'')+'</div>'+
      (manips?'<div class="section"><h3>Манипуляции</h3>'+manips+'</div>':'')+
      (issues?'<div class="section"><h3>Логические ошибки</h3>'+issues+'</div>':'')+
      '<div class="footer"><a href="/">Проверить свою статью →</a><span class="badge">Поделились результатом</span></div>';
  })
  .catch(()=>{document.getElementById('content').innerHTML='<div style="color:#ef4444">Ссылка устарела или не существует</div>';});
</script>
</body>
</html>`))

// ShowPage — GET /s/:id → HTML share page
func (h *ShareHandler) ShowPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	shareTmpl.Execute(w, nil)
}
