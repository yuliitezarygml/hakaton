# 🔍 ANALYST — Text & Disinformation Analyzer

> AI-powered fact-checking platform: detects disinformation, manipulations, and logical fallacies in articles and news.  
> Works as a **web app**, **Telegram bot**, and **Chrome extension** — all connected to the same backend.

---

## 📐 Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         INTERNET / CLIENTS                      │
│  ┌──────────────┐  ┌──────────────────┐  ┌──────────────────┐  │
│  │ Chrome Ext.  │  │  Telegram Bot    │  │  Web Browser     │  │
│  │ (Manifest V3)│  │  (Go, polling/   │  │  (Fact Guard     │  │
│  │              │  │   webhook)       │  │   frontend)      │  │
│  └──────┬───────┘  └────────┬─────────┘  └────────┬─────────┘  │
└─────────┼───────────────────┼─────────────────────┼────────────┘
          │                   │                     │
          ▼                   ▼                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                     NGINX (port 80)                             │
│  /api/*  → backend:8080                                         │
│  /admin/ → backend:8080                                         │
│  /s/*    → backend:8080  (share pages)                          │
│  /       → fact-guard:80 (frontend)                             │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                  GO BACKEND (port 8080)                         │
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────────────┐   │
│  │  Analyzer   │  │   Content   │  │   Serper (Google     │   │
│  │  Service    │  │   Fetcher   │  │   Search API)        │   │
│  │             │  │             │  │                      │   │
│  │ • Queue(1)  │  │ • HTML parse│  │ • Fact verification  │   │
│  │ • Pause/    │  │ • SPA fbk   │  │ • Multi-lang search  │   │
│  │   Resume    │  │ • OG/ld+json│  │ • Cross-check        │   │
│  └──────┬──────┘  └─────────────┘  └──────────────────────┘   │
│         │                                                       │
│  ┌──────▼──────────────────────────────────┐                   │
│  │           AI Client (interface)          │                   │
│  │  ┌────────────┐      ┌───────────────┐  │                   │
│  │  │    Groq    │  OR  │  OpenRouter   │  │                   │
│  │  │ llama-3.3  │      │ qwen3/deepseek│  │                   │
│  │  └────────────┘      └───────────────┘  │                   │
│  └─────────────────────────────────────────┘                   │
└────────────┬────────────────────┬───────────────────────────────┘
             │                    │
      ┌──────▼──────┐     ┌───────▼──────┐
      │  PostgreSQL  │     │    Redis     │
      │  (results,   │     │  (24h cache, │
      │   shares,    │     │   hash-key)  │
      │   domains)   │     └──────────────┘
      └──────────────┘
```

---

## 🗂️ Project Structure

```
openrouter-web/
├── main.go                    # HTTP server, route registration
├── Dockerfile                 # Go backend image
├── docker-compose.yml         # All 6 services
├── nginx.conf                 # Reverse proxy config
├── .env.example               # All env vars documented
│
├── handlers/                  # HTTP handlers
│   ├── analyzer.go            # /api/analyze, /api/analyze/stream, /api/chat
│   ├── share.go               # /api/share (create), /s/:id (view page)
│   ├── admin.go               # /api/admin/* (stats, logs, pause/resume)
│   ├── docker.go              # /api/admin/docker/* (containers, logs WS)
│   └── domain.go              # /api/domain/:host, /api/domains/top
│
├── services/                  # Business logic
│   ├── analyzer.go            # Analysis pipeline: cache→fetch→AI→verify→save
│   ├── fetcher.go             # Smart URL fetcher (HTML, SPA, OG tags)
│   ├── openrouter.go          # OpenRouter AI client
│   ├── groq.go                # Groq AI client (faster, free)
│   ├── serper.go              # Google Search via Serper API
│   ├── ratelimit.go           # Rate limit tracker per provider
│   ├── prompt_loader.go       # Loads prompts from config/prompts.json
│   └── domain.go              # Domain reputation stats
│
├── config/
│   └── prompts.json           # AI system prompt, scoring rules, examples
│
├── database/                  # PostgreSQL init, connection
├── cache/                     # Redis client wrapper
├── models/                    # Shared Go structs (AnalysisResponse, etc.)
├── logger/                    # Ring-buffer log writer for admin panel
│
├── admin/                     # Admin panel (static HTML)
│   ├── index.html             # Dashboard: stats, logs, rate limits
│   ├── docker.html            # Docker containers management
│   ├── share.html             # Public share result page
│   └── mian.css               # Shared admin design system
│
├── EXTENSION/                 # Chrome Extension (Manifest V3)
│   ├── manifest.json
│   ├── popup.html / popup.js  # Extension popup UI
│   ├── popup.css              # Premium glassmorphism design
│   ├── content.js             # Page scanner (injected into tab)
│   └── background.js          # Service worker
│
├── telegram-bot/              # Telegram bot (separate Go module)
│   ├── main.go                # Bot logic: polling/webhook, handlers
│   ├── analyzer.go            # SSE client for /api/analyze/stream
│   └── formatter.go           # Telegram HTML message formatter
│
└── Fact_Guard-main/           # Web frontend (separate service)
    └── ...                    # React/Next.js frontend app
```

---

## 🚀 How It Works — Full Pipeline

### URL Analysis Flow

```
User sends URL
      │
      ▼
1. Content Fetcher
   ├── HTTP GET with browser-like headers
   ├── Parse HTML → extract main text
   ├── Fallback: ld+json structured data
   ├── Fallback: OG meta tags (title + description)
   └── Result: clean article text

      │
      ▼
2. Redis Cache Check (SHA-256 hash of text)
   ├── HIT  → return cached result instantly
   └── MISS → continue pipeline

      │
      ▼
3. Serper Web Search (optional, if SERPER_API_KEY set)
   └── Search Google for key claims in article
       → adds "INTERNET CONTEXT" block to AI prompt

      │
      ▼
4. Request Queue (semaphore, max 1 concurrent AI request)
   └── Other requests wait with position indicator

      │
      ▼
5. AI Analysis (Groq or OpenRouter)
   └── Sends: system_prompt + article text + search context
       Receives: JSON with:
         • credibility_score (0-10)
         • summary
         • manipulations[]
         • logical_issues[]
         • fact_check { verifiable_facts, opinions_as_facts, missing_evidence }
         • score_breakdown (step-by-step)
         • final_verdict
         • reasoning

      │
      ▼
6. Cross-Verification (if score ≤ 7 and Serper available)
   └── Search for key claims in multiple languages
       → adds real_information and verified_sources

      │
      ▼
7. Save results
   ├── Redis cache (24h, SHA-256 key)
   └── PostgreSQL (analysis_results table)

      │
      ▼
8. Stream back to client via SSE
   Events: start → progress → progress → ... → result
```

---

## 📡 API Reference

### Analysis

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/analyze` | Full analysis, returns JSON |
| `POST` | `/api/analyze/stream` | SSE stream: `start`, `progress`, `result`, `error` |
| `POST` | `/api/chat` | Chat with AI about analysis context |
| `GET`  | `/api/health` | Health check → `{"status":"ok"}` |
| `GET`  | `/api/limits` | Rate limit stats per AI provider |

**Request body** (`/api/analyze`, `/api/analyze/stream`):
```json
{ "url": "https://example.com/article" }
// OR
{ "text": "Article text (min 100 chars)..." }
```

**Result JSON structure**:
```json
{
  "credibility_score": 3,
  "summary": "Article summary...",
  "manipulations": ["Emotional language: phrase X", "..."],
  "logical_issues": ["False cause-effect: ...", "..."],
  "fact_check": {
    "verifiable_facts": ["..."],
    "opinions_as_facts": ["..."],
    "missing_evidence": ["..."]
  },
  "score_breakdown": "Started at 5/10: -1 for emotional language, -1 for missing sources = 3/10",
  "final_verdict": "FAKE",
  "reasoning": "...",
  "verification": {
    "is_fake": true,
    "fake_reasons": ["3 manipulations found", "..."],
    "real_information": "Real info from verified sources...",
    "verified_sources": [{"title":"...", "url":"...", "description":"..."}]
  }
}
```

### Sharing

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/share` | Save result → returns `{"id":"abc123","url":"https://.../s/abc123"}` |
| `GET`  | `/api/share/:id` | Get raw JSON result from DB |
| `GET`  | `/s/:id` | Beautiful HTML share page |

### Domain Stats

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET`  | `/api/domain/:host` | Domain reputation stats |
| `GET`  | `/api/domains/top` | Top analyzed domains |

### Admin (requires `X-Admin-Token` header)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET`  | `/api/admin/stats` | Analysis counts, recent results |
| `GET`  | `/api/admin/logs` | SSE live log stream |
| `POST` | `/api/admin/pause` | Pause analysis processing |
| `POST` | `/api/admin/resume` | Resume analysis processing |
| `GET`  | `/api/admin/docker/containers` | List Docker containers |
| `POST` | `/api/admin/docker/action` | Start/stop/restart container |
| `WS`   | `/api/admin/docker/logs` | WebSocket container log stream |

---

## 🤖 Telegram Bot

The bot connects to the same backend via `/api/analyze/stream`.

### Supported inputs
| Input | Action |
|-------|--------|
| URL (`https://...`) | Fetch & analyze the article |
| Forwarded message with URL | Extract URL, show source label |
| Plain text | Politely respond: "URL analysis only, text in development" |

### Commands
| Command | Description |
|---------|-------------|
| `/start` | Welcome message |
| `/help` | Usage instructions |
| `/cancel` | Stop current analysis |

### Result message format
```
🟡 4/10 — СОМНИТЕЛЬНО
[████░░░░░░] 4/10

📝 Article summary text...

⚠️ Манипуляции:
• Emotional language: phrase X
• Appeal to emotions instead of facts

🔍 Логические ошибки:
• False cause-effect: ...

[🔗 Поделиться] [🔄 Перепроверить]
```

### Deployment modes
- **Polling** (default, local dev) — long-polling Telegram API
- **Webhook** (production) — set `WEBHOOK_URL` in `.env`

---

## 🧩 Chrome Extension

**Manifest V3** extension that analyzes the current browser tab.

### Flow
```
User clicks extension icon
        │
        ▼
1. Health check → backend online?
        │
        ▼
2. Content script injected into tab
   → Extracts text from page DOM
   → Sends chunks back to popup via chrome.runtime.onMessage
        │
        ▼
3. Popup shows scan progress
   (chunks, char count, progress bar)
        │
        ▼
4. POST /api/analyze/stream with tab URL
   → SSE stream → live event log
        │
        ▼
5. Render result:
   • Score (0-10) with color
   • Verdict badge
   • Summary
   • Manipulations list
   • Logical issues list
   • Missing evidence
```

### Permissions
- `activeTab` — access current tab
- `scripting` — inject content.js
- `storage` — cache, history, userId
- `contextMenus` — right-click menu
- `host_permissions`: `https://apich.sinkdev.dev/*`

---

## 🛡️ Admin Panel

Accessible at `/admin/` (requires token).

### Dashboard (`index.html`)
- Total analyses count
- Fake/credible ratio pie chart  
- Recent analyses table (URL, score, verdict, date)
- Live WebSocket log stream
- API Tester (send test requests)
- Rate limits display (`/api/limits`)

### Docker Manager (`docker.html`)
- List all Docker containers with status
- Start / Stop / Restart any container
- Real-time WebSocket log stream per container

### Share Page (`share.html`)
- Public — no auth required
- Fetches result from DB via `/api/share/:id`
- Animated score counter (0→N)
- Color-coded verdict
- Manipulations, logical issues, missing evidence panels

---

## 🧠 AI Scoring System

The prompt instructs the AI with strict rules (in `config/prompts.json`):

```
START:  5/10 (neutral)

DEDUCTIONS:
  -0.5  each identified manipulation (with quote)
  -0.5  each claim without evidence
  -0.5  each opinion presented as fact
  -1.0  internal logical contradiction
  -1.0  emotional / alarmist language
  -1.0  no sources cited at all
  -1.0  misleading or sensationalist title
  -1.0  unverifiable or partisan sources
  -2.0  demonstrable disinformation

ADDITIONS:
  +0.5  verified fact with primary source citation
  +1.0  multiple independent sources cited
  +1.0  official document or peer-reviewed study

ANTI-INFLATION RULES:
  8+  ONLY for: peer-reviewed, official government docs with sources
  7   ONLY if: verified facts, neutral tone, max 1 minor issue
  ≤6  All regular news/blog articles
  ≠8+ if site name = "verified" / "provereno" — content is what matters
```

---

## 🐳 Docker Services

| Service | Image | Purpose |
|---------|-------|---------|
| `backend` | Custom Go build | Main API server |
| `telegram-bot` | Custom Go build | Telegram bot |
| `postgres` | `postgres:15-alpine` | Persistent storage |
| `redis` | `redis:7-alpine` | Analysis cache |
| `fact-guard` | Custom build | Web frontend |
| `nginx` | `nginx:alpine` | Reverse proxy, port 80 |

### Health Checks
- **postgres**: `pg_isready` every 10s
- **backend**: `GET /api/health` every 15s
- **telegram-bot**: waits for `backend` to be healthy

---

## ⚙️ Environment Variables

```env
# AI Providers
USE_GROQ=true
GROQ_API_KEY=gsk_...
GROQ_MODEL=llama-3.3-70b-versatile

OPENROUTER_API_KEY=sk-or-v1-...
OPENROUTER_MODEL=qwen/qwen3-coder:free
OPENROUTER_MODEL_BACKUP=deepseek/deepseek-r1-0528:free

# Web Search (Serper)
SERPER_API_KEY=...

# Server
PORT=8080
ADMIN_TOKEN=your_secret_token

# Database
DB_URL=postgres://user:password@postgres:5432/text_analyzer?sslmode=disable
REDIS_URL=redis:6379

# Telegram Bot
TELEGRAM_TOKEN=your_bot_token
API_BASE=https://your-domain.com

# Webhook mode (optional, falls back to polling)
# WEBHOOK_URL=https://your-domain.com
# WEBHOOK_PORT=8443
```

---

## 🚀 Quick Start

```bash
# 1. Clone and configure
cp .env.example .env
# Edit .env with your API keys

# 2. Start all services
docker compose up -d

# 3. Check status
docker compose ps
docker compose logs -f backend

# 4. Access
# Web app:    http://localhost/
# Admin:      http://localhost/admin/
# API health: http://localhost/api/health
```

---

## 🔄 NGINX Routing

```nginx
location /api/   → backend:8080   # All API endpoints
location /admin/ → backend:8080   # Admin panel static files
location /s/     → backend:8080   # Share result pages
location /       → fact-guard:80  # Web frontend (catch-all)
```

---

## 📊 Database Schema (PostgreSQL)

```sql
-- Analysis results
CREATE TABLE analysis_results (
  id         SERIAL PRIMARY KEY,
  text       TEXT,
  url        TEXT,
  result     JSONB,
  created_at TIMESTAMP DEFAULT NOW()
);

-- Shared results (with expiry)
CREATE TABLE shared_results (
  id         VARCHAR(12) PRIMARY KEY,
  result     JSONB,
  created_at TIMESTAMP DEFAULT NOW(),
  expires_at TIMESTAMP DEFAULT NOW() + INTERVAL '30 days'
);

-- Domain reputation
CREATE TABLE domain_stats (
  domain     TEXT PRIMARY KEY,
  total      INT,
  avg_score  FLOAT,
  last_seen  TIMESTAMP
);
```

---

## 📋 Task History (All Completed Tasks)

### Backend

- [x] **Content Fetcher (SPA)** — fallbacks for JS sites: ld+json → OG meta tags (`services/fetcher.go`)
- [x] **Rate limit tracking** — reads `Retry-After` / `X-RateLimit-Reset-Requests` from 429 responses, logs wait time
- [x] **GET /api/limits** — endpoint with current rate limit data per AI provider (`services/ratelimit.go`)
- [x] **Request queue** — semaphore (max 1 concurrent AI request), position indicator to users
- [x] **Redis caching** — 24h cache by SHA-256 hash of content
- [x] **PostgreSQL persistence** — all results saved with URL and timestamp
- [x] **Cross-verification** — Serper multi-language search to verify key claims
- [x] **Pausing** — admin can pause/resume all analysis (`IsPaused` atomic flag)
- [x] **Chat endpoint** — `/api/chat` with analysis context passed to AI

### Admin Panel

- [x] **Full redesign** — IBM Plex Mono + Bebas Neue, grid background, CRT scanlines, amber theme
- [x] **Live log stream** — WebSocket log feed in real time (`/api/admin/logs`)
- [x] **API Tester** — test API requests directly from admin panel
- [x] **Docker manager** — list containers, start/stop/restart, WebSocket log stream per container (`admin/docker.html`)
- [x] **Rate limits display** — visualize `/api/limits` data with progress bars

### Chrome Extension

- [x] **userId + history** — UUID per user, stores 30 entries for 7 days, view with 🕐 button
- [x] **Auto-scan without animation** — silently scans on popup open, animation only on manual click
- [x] **Cache** — same page → instant result from cache, 🔄 button for force rescan
- [x] **Floating notification** — result shown as a floating div in bottom-right corner of page
- [x] **Premium redesign** — glassmorphism, IBM Plex Mono, vibrant colors, micro-animations
- [x] **Health check** — backend status indicator, disables button if offline

### Telegram Bot

- [x] **Separate Go module** — `telegram-bot/` directory with `main.go`, `analyzer.go`, `formatter.go`
- [x] **SSE client** — streams `/api/analyze/stream`, edits Telegram message in progress
- [x] **Message formatting** — score + progress bar + verdict + manipulations + logical issues
- [x] **Multi-user support** — each chat has independent analysis context and cancel
- [x] **/cancel command** — stop current analysis per chat
- [x] **Webhook mode** — auto-selected when `WEBHOOK_URL` is set in `.env`
- [x] **Inline keyboard** — 🔗 Share and 🔄 Re-check buttons on result
- [x] **Forwarded messages** — detects source channel/user, shows as label in result
- [x] **URL-only mode** — plain text shows polite "in development" message
- [x] **Gemini removed** — removed video analysis dependency, simplified to URL-only

### Infrastructure

- [x] **Single .env** — bot reads `../.env` from project root, shared `.env.example`
- [x] **Docker Compose** — nginx (SSE buffering fixed, timeouts), telegram-bot service, `env_file`, postgres + backend healthchecks
- [x] **nginx share route** — added `location /s/` → backend (was missing, causing 502)
- [x] **Share page HTML** — premium `admin/share.html` matching admin design, animated score counter, fetches from DB
- [x] **Prompt improvements** — anti-authority bias rule (site name ≠ credibility score), multilingual analysis support

### AI Prompt System

- [x] **Strict scoring rules** — starts at 5/10, deductions for each issue, 8+ only for peer-reviewed
- [x] **Anti-inflation examples** — calibration examples of wrong vs correct scoring
- [x] **Step-by-step breakdown** — AI must justify every +/- with quote from text
- [x] **Score calibration** — `0-2` propaganda, `3-4` mostly false, `5-6` mixed, `7` credible, `8-9` high credibility, `10` peer-reviewed only
