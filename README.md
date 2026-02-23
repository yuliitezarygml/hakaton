# Text Analyzer

Instrument pentru verificarea textelor și articolelor în vederea detectării dezinformării, manipulărilor și erorilor logice.

**Trei componente:** Backend Go · Interfață Next.js · Extensie Chrome

---

## Arhitectura sistemului

```
┌────────────────────────────────────────────────────────────────────────┐
│                           UTILIZATOR                                   │
└─────────────────────┬──────────────────────────┬───────────────────────┘
                      │                          │
                      ▼                          ▼
      ┌───────────────────────────┐   ┌──────────────────────────────┐
      │     Chrome Extension      │   │      Next.js Frontend         │
      │                           │   │      localhost:3000            │
      │  • Buton în toolbar       │   │                               │
      │  • Scanare pagină cu      │   │  • Câmp pentru text / URL     │
      │    animație               │   │  • Log live de evenimente     │
      │  • Context menu           │   │  • Card cu rezultatul         │
      │    (text selectat)        │   └───────────────┬───────────────┘
      └─────────────┬─────────────┘                   │
                    │                     /api/* proxy │
                    │   POST { url }                   │
                    └───────────────────┬──────────────┘
                                        │
                                        ▼
┌───────────────────────────────────────────────────────────────────────┐
│                       Go Backend  :8080                               │
│                                                                       │
│   POST /api/analyze/stream   ──  SSE (flux de evenimente)            │
│   POST /api/analyze          ──  JSON sincron                        │
│   GET  /api/health           ──  verificare disponibilitate          │
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │                      AnalyzerService                             │ │
│  │                                                                  │ │
│  │  PAS 1  URL → ContentFetcher ──► descarcă HTML                 │ │
│  │                                   parsează, elimină zgomotul    │ │
│  │                                   → text curat                  │ │
│  │                                                                  │ │
│  │  PAS 2  Serper API ──────────► caută fapte în Google           │ │
│  │         (dacă e configurat)      RO + EN + RU                   │ │
│  │                                  adaugă context la text         │ │
│  │                                                                  │ │
│  │  PAS 3  Text + context ─────► furnizor AI                      │ │
│  │                                  prompt din prompts.json        │ │
│  │                                  → JSON cu evaluări             │ │
│  │                                                                  │ │
│  │  PAS 4  scor ≤ 7 ───────────► verificare prin Serper           │ │
│  │                                  3 interogări pe afirmații      │ │
│  │                                  cheie                          │ │
│  └──────────────────────────────────┬────────────────────────────── ┘ │
└─────────────────────────────────────┼───────────────────────────────┘
                                      │
               ┌──────────────────────┴────────────────────┐
               │                                           │
               ▼                                           ▼
   ┌───────────────────────┐                 ┌────────────────────────┐
   │    Furnizor AI        │                 │      Serper API         │
   │                       │                 │   (Google Search)       │
   │  • Groq               │                 │                         │
   │    llama-3.3-70b      │                 │  • Căutare fapte       │
   │  • OpenRouter         │                 │  • Limbi: RO EN RU     │
   │    qwen / deepseek    │                 │  • Verificare          │
   │  • LM Studio          │                 │    încrucișată         │
   │    model local        │                 └────────────────────────┘
   └───────────────────────┘
```

---

## Fluxul datelor

```
  Cerere { text: "..." } sau { url: "..." }
          │
          ▼
  ┌───────────────────────────────────────────────────────────┐
  │ PAS 1 — Obținerea textului                                │
  │   Dacă URL:                                               │
  │     → HTTP GET pagina                                     │
  │     → Parsare HTML (golang.org/x/net/html)               │
  │     → Se elimină: nav header footer aside scripturi       │
  │       reclame cookie-bannere widgeturi iframe             │
  │     → Maximum 15 000 de caractere                        │
  └─────────────────────────────┬─────────────────────────────┘
                                │
                                ▼
  ┌───────────────────────────────────────────────────────────┐
  │ PAS 2 — Căutare context (dacă SERPER_API_KEY e setat)    │
  │   → 3 interogări de căutare paralele                     │
  │   → Limbi: română + engleză + rusă                       │
  │   → Top-3 rezultate per interogare                       │
  │   → Contextul se adaugă la text înainte de analiză       │
  └─────────────────────────────┬─────────────────────────────┘
                                │
                                ▼
  ┌───────────────────────────────────────────────────────────┐
  │ PAS 3 — Analiză prin LLM                                 │
  │   → Text + context → cerere API                          │
  │   → Promptul definește algoritmul de analiză în 8 pași   │
  │   → Răspuns — JSON strict                                │
  └─────────────────────────────┬─────────────────────────────┘
                                │
                                ▼
  ┌───────────────────────────────────────────────────────────┐
  │ PAS 4 — Verificare încrucișată (dacă scor ≤ 7)           │
  │   → Din analiză se extrag afirmațiile contestabile       │
  │   → Serper caută infirmări sau confirmări                │
  │   → Sursele găsite se adaugă la răspuns                  │
  └─────────────────────────────┬─────────────────────────────┘
                                │
                                ▼
  AnalysisResponse {
    credibility_score       — scor 0..10
    summary                 — concluzie scurtă
    manipulations[]         — lista manipulărilor găsite
    logical_issues[]        — erori logice
    fact_check {
      verifiable_facts[]    — afirmații verificabile
      opinions_as_facts[]   — opinii prezentate ca fapte
      missing_evidence[]    — afirmații fără dovezi
    }
    verification {
      is_fake               — true / false
      fake_reasons[]        — motive
      real_information      — ce s-a găsit pe internet
      verified_sources[]    — linkuri spre surse
    }
  }
```

---

## Extensie Chrome — schema de funcționare

```
  MODUL 1 — Buton în toolbar
  ─────────────────────────────────────────────────────────────────
  Click pe iconiță → se deschide popup.html
          │
          ▼
  Verificare backend: GET /api/health
          │
          ▼
  Click "Analizează" → content.js se injectează în tab
          │
          ▼
  Animație content.js (3 sec):
    • Overlay întunecat semitransparent peste toată pagina
    • Linie albastră luminoasă — fixată la 35% din înălțimea ecranului
    • Pagina se derulează automat sub ea
    • h1..h4, p, blockquote se evidențiază la trecerea liniei
    • nav / footer / reclame / cookie-bannere — sunt ignorate
    • La final pagina revine la poziția inițială
          │
          ▼
  scan_done → popup primește semnalul
          │
          ▼
  POST /api/analyze/stream { url: url_curent }
          │
          ▼
  Flux SSE → popup afișează log în timp real
          │
          ▼
  event: result → card cu scorul 🔴 / 🟡 / 🟢 + detalii


  MODUL 2 — Context Menu (text selectat)
  ─────────────────────────────────────────────────────────────────
  Selectează text → click dreapta → "Analizează: «...»"
          │
          ▼
  background.js (service worker):
    → salvează textul în chrome.storage.session
    → apelează chrome.windows.create
          │
          ▼
  Se deschide popup.html?autostart=1 (fereastră 440×600)
          │
          ▼
  popup.js citește pendingText → imediat:
  POST /api/analyze/stream { text: text_selectat }
          │
          ▼
  SSE → rezultat (fără faza de scanare)
```

---

## Evenimente SSE

| Eveniment  | Date                              | Culoare UI      |
|------------|-----------------------------------|-----------------|
| `start`    | Mesaj de început                  | 🔵 albastru     |
| `progress` | Pas de execuție                   | 🟢 verde        |
| `result`   | JSON cu rezultatul complet        | 🟣 violet       |
| `done`     | Mesaj de finalizare               | 🟠 portocaliu   |
| `error`    | Descrierea erorii                 | 🔴 roșu         |

---

## Scara de credibilitate

| Scor   | Verdict                            | Indicator |
|--------|------------------------------------|-----------|
| 0 – 3  | Probabilitate ridicată de fals     | 🔴        |
| 4 – 6  | Conținut îndoielnic                | 🟡        |
| 7 – 10 | Conținut pare credibil             | 🟢        |

Scor de bază — 5. Fiecare manipulare −0.5, eroare logică −1, fapt cu sursă +0.5.

---

## Structura proiectului

```
openrouter-web/
│
├── main.go                       # Punct de intrare: configurare, rutare, pornire
│
├── config/
│   ├── config.go                 # Încărcare variabile .env
│   └── prompts.json              # Prompturi de sistem pentru LLM
│
├── handlers/
│   └── analyzer.go               # Handlere HTTP (/stream, /analyze, /health)
│
├── models/
│   └── analysis.go               # Structuri de cerere și răspuns
│
├── services/
│   ├── analyzer.go               # Orchestrator: 4 pași de analiză
│   ├── fetcher.go                # Descărcare și parsare HTML după URL
│   ├── groq.go                   # Client Groq API
│   ├── openrouter.go             # Client OpenRouter (+ model de rezervă)
│   ├── lmstudio.go               # Client LM Studio (modele locale)
│   ├── serper.go                 # Client Google Search (Serper API)
│   └── prompt_loader.go          # Încărcare prompturi din JSON
│
├── frontend/                     # Next.js 16 + React 19 + Tailwind v4
│   ├── app/
│   │   ├── page.js               # Pagina principală ('use client')
│   │   ├── layout.js
│   │   └── globals.css           # Temă întunecată
│   ├── components/
│   │   ├── InputForm.js          # Câmp de introducere text sau URL
│   │   ├── EventLog.js           # Container log evenimente
│   │   ├── EventRow.js           # Rând log: badge + text
│   │   ├── ResultCard.js         # Card cu rezultatul analizei
│   │   └── StatusBar.js          # Indicator conexiune cu backend
│   ├── hooks/
│   │   └── useAnalyzer.js        # SSE prin fetch + ReadableStream
│   └── next.config.mjs           # Proxy /api/* → localhost:8080
│
└── extension/                    # Extensie Chrome Manifest V3
    ├── manifest.json
    ├── background.js             # Service Worker: meniu contextual
    ├── content.js                # Injectat în pagină: animație scanare
    ├── popup.html                # UI: 4 stări (idle/scan/analyze/result)
    ├── popup.css                 # Temă întunecată
    └── popup.js                  # Logică: health → scan → SSE → rezultat
```

---

## Pornire rapidă

### Cerințe

- Go 1.21+
- Node.js 18+
- Cheie API: [Groq](https://console.groq.com) (gratuit) sau OpenRouter / LM Studio

### 1. Configurare .env

```bash
cp .env.example .env
```

```env
# Groq — rapid și gratuit (implicit)
USE_GROQ=true
GROQ_API_KEY=gsk_...
GROQ_MODEL=llama-3.3-70b-versatile

# OpenRouter (USE_GROQ=false)
OPENROUTER_API_KEY=sk-or-...
OPENROUTER_MODEL=qwen/qwen3-coder:free
OPENROUTER_MODEL_BACKUP=deepseek/deepseek-r1-0528:free

# LM Studio — local, fără internet (USE_LM_STUDIO=true)
LM_STUDIO_URL=http://localhost:1234
LM_STUDIO_MODEL=local-model

# Serper — căutare fapte în Google (opțional)
SERPER_API_KEY=...

PORT=8080
```

### 2. Pornire backend

```bash
go run main.go
# → http://localhost:8080
```

### 3. Pornire frontend

```bash
cd frontend
npm install
npm run dev
# → http://localhost:3000
```

### 4. Instalare extensie

1. Deschide `chrome://extensions/`
2. Activează **Modul dezvoltator**
3. **Încarcă extensie nepachetată** → selectează folderul `extension/`
4. Asigură-te că backend-ul rulează pe `localhost:8080`

---

## API

### `POST /api/analyze/stream` — SSE

```json
{ "url": "https://example.com/article" }
```
```json
{ "text": "text pentru analiză..." }
```

### `POST /api/analyze` — sincron

Aceleași câmpuri. Returnează JSON complet fără streaming.

### `GET /api/health`

```json
{ "status": "ok" }
```

---

## Furnizori AI

| Furnizor    | Variabilă              | Model implicit                | Particularități              |
|-------------|------------------------|-------------------------------|------------------------------|
| Groq        | `USE_GROQ=true`        | llama-3.3-70b-versatile       | Gratuit, foarte rapid        |
| OpenRouter  | _(implicit)_           | qwen/qwen3-coder:free         | Multe modele + failover      |
| LM Studio   | `USE_LM_STUDIO=true`   | local-model                   | Local, fără cheie API        |
