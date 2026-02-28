from fastapi import FastAPI, Request, Response
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates
from fastapi.responses import HTMLResponse, JSONResponse, StreamingResponse
from starlette.middleware.base import BaseHTTPMiddleware
from config import Config
import os
import time
import json
import httpx

# Поддерживаемые языки
SUPPORTED_LANGUAGES = ['ru', 'en', 'ro']
DEFAULT_LANGUAGE = 'en'

# Загрузка переводов
def load_translations():
    translations = {}
    for lang in SUPPORTED_LANGUAGES:
        try:
            with open(f'locales/{lang}.json', 'r', encoding='utf-8') as f:
                translations[lang] = json.load(f)
        except FileNotFoundError:
            print(f"Warning: Translation file for '{lang}' not found")
            translations[lang] = {}
    return translations

TRANSLATIONS = load_translations()

def get_translation(lang: str, key: str, default: str = None):
    """Получить перевод по ключу (поддержка вложенных ключей через точку)"""
    if lang not in TRANSLATIONS:
        lang = DEFAULT_LANGUAGE
    
    keys = key.split('.')
    value = TRANSLATIONS.get(lang, {})
    
    for k in keys:
        if isinstance(value, dict):
            value = value.get(k)
        else:
            return default or key
    
    return value if value is not None else (default or key)

def get_lang_from_request(request: Request) -> str:
    """Получить язык из cookie или использовать язык по умолчанию"""
    lang = request.cookies.get('lang', DEFAULT_LANGUAGE)
    if lang not in SUPPORTED_LANGUAGES:
        lang = DEFAULT_LANGUAGE
    return lang

# Middleware для отключения кэширования
class NoCacheMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request, call_next):
        response = await call_next(request)
        response.headers["Cache-Control"] = "no-cache, no-store, must-revalidate"
        response.headers["Pragma"] = "no-cache"
        response.headers["Expires"] = "0"
        return response

# Инициализация приложения
app = FastAPI(title="Fact Guard")

# Добавление middleware для отключения кэширования
app.add_middleware(NoCacheMiddleware)

# Настройка шаблонов Jinja2 с контекстом и отключением кэша в разработке
templates = Jinja2Templates(directory="templates")

# Отключить кэширование только в режиме разработки (PROD=false)
if not Config.DEBUG:
    templates.env.cache = None  # Отключить кэширование шаблонов
    templates.env.auto_reload = True  # Включить автоперезагрузку при изменении

templates.env.globals["timestamp"] = lambda: int(time.time())

# Добавляем функцию перевода в глобальные переменные шаблонов
def t(key: str, lang: str = DEFAULT_LANGUAGE, default: str = None):
    """Функция перевода для использования в шаблонах"""
    return get_translation(lang, key, default)

templates.env.globals["t"] = t
templates.env.globals["SUPPORTED_LANGUAGES"] = SUPPORTED_LANGUAGES
templates.env.globals["TRANSLATIONS"] = TRANSLATIONS

# Фильтр для обрезки текста до N символов
def truncate_text(text, length=250):
    if len(text) > length:
        return text[:length] + "..."
    return text

templates.env.filters["truncate"] = truncate_text

# Подключение статических файлов
app.mount("/static", StaticFiles(directory="static"), name="static")

ANALYZE_STREAM_URL = os.getenv("ANALYZE_STREAM_URL", "https://apich.sinkdev.dev/api/analyze/stream")
ADMIN_STATS_URL = os.getenv("ADMIN_STATS_URL", "https://apich.sinkdev.dev/api/admin/stats")
ADMIN_TOKEN = os.getenv("ADMIN_TOKEN", "admin_secret_123")

FAKE_API_RESULT = {
        "result": "ok",
        "url": "https://provereno.media/blog/2026/02/25/pravda-li-chto-yoko-ono-rodstvennitsa-pushkina/",
        "summary": "Articolul prezintă informații despre viața și familia Yoko Ono, dar nu reușește să stabilească o legătură clară între ea și Pușkin.",
        "source_url": "https://provereno.media/blog/2026/02/25/pravda-li-chto-yoko-ono-rodstvennitsa-pushkina/",
        "fact_check": {
                "verifiable_facts": [
                        "Yoko Ono s-a născut în Tokyo, într-o familie bogată.",
                        "Tatăl ei, Eiichi Ono, a fost un bancher.",
                        "Yoko Ono a studiat la o școală din Tokyo și a fost influențată de arta și muzica occidentală."
                ],
                "opinions_as_facts": [
                        "Articolul prezintă informații despre viața și familia Yoko Ono, dar nu oferă dovezi concrete pentru a susține legătura de rudenie cu Pușkin."
                ],
                "missing_evidence": [
                        "Nu există dovezi concrete care să susțină legătura de rudenie între Yoko Ono și Alexander Pușkin."
                ]
        },
        "manipulations": [
                "Articolul folosește un titlu senzaționalist care sugerează o legătură de rudenie între Yoko Ono și Pușkin, dar nu oferă dovezi concrete pentru a susține această afirmație."
        ],
        "logical_issues": [
                "Articolul prezintă o serie de informații despre viața și familia Yoko Ono, dar nu reușește să stabilească o legătură clară între ea și Pușkin."
        ],
        "credibility_score": 8,
        "score_breakdown": "pornind de la 5/10, +3 pentru informații verificabile, -0,5 pentru lipsa de dovezi concrete = 7,5/10",
        "final_verdict": "PARȚIAL ADEVĂRAT",
        "verdict_explanation": "Articolul prezintă informații verificabile despre viața și familia Yoko Ono, dar nu oferă dovezi concrete pentru a susține legătura de rudenie cu Pușkin. Prin urmare, verdictul final este PARȚIAL ADEVĂRAT.",
        "reasoning": "Am început cu un scor de 5/10, dar am adăugat 3 puncte pentru că articolul prezintă informații verificabile despre viața și familia Yoko Ono. Totuși, am scăzut scorul cu 0,5 puncte pentru că articolul nu oferă dovezi concrete pentru a susține legătura de rudenie cu Pușkin.",
        "sources": [
                {
                        "title": "Wikipedia",
                        "url": "https://ru.wikipedia.org/wiki/%D0%9E%D0%BD%D0%BE,_%D0%99%D0%BE%D0%BA%D0%BE",
                        "description": "Informații despre viața și familia Yoko Ono"
                }
        ],
        "verification": {
                "is_fake": False,
                "fake_reasons": [
                        "Lipsa de dovezi concrete pentru a susține legătura de rudenie între Yoko Ono și Pușkin."
                ],
                "real_information": "Yoko Ono s-a născut în Tokyo, într-o familie bogată, și a studiat la o școală din Tokyo.",
                "verified_sources": [
                        "Wikipedia"
                ]
        },
        "usage": {
                "prompt_tokens": 5549,
                "completion_tokens": 828,
                "total_tokens": 6377
        },
        "raw_response": """```json
{
    \"credibility_score\": 8,
    \"fact_check\": {
        \"missing_evidence\": [
            \"Nu există dovezi concrete care să susțină legătura de rudenie între Yoko Ono și Alexander Pușkin.\"
        ],
        \"opinions_as_facts\": [
            \"Articolul prezintă informații despre viața și familia Yoko Ono, dar nu oferă dovezi concrete pentru a susține legătura de rudenie cu Pușkin.\"
        ],
        \"verifiable_facts\": [
            \"Yoko Ono s-a născut în Tokyo, într-o familie bogată.\",
            \"Tatăl ei, Eiichi Ono, a fost un bancher.\",
            \"Yoko Ono a studiat la o școală din Tokyo și a fost influențată de arta și muzica occidentală.\"
        ]
    },
    \"final_verdict\": \"PARȚIAL ADEVĂRAT\",
    \"logical_issues\": [
        \"Articolul prezintă o serie de informații despre viața și familia Yoko Ono, dar nu reușește să stabilească o legătură clară între ea și Pușkin.\"
    ],
    \"manipulations\": [
        \"Articolul folosește un titlu senzaționalist care sugerează o legătură de rudenie între Yoko Ono și Pușkin, dar nu oferă dovezi concrete pentru a susține această afirmație.\"
    ],
    \"reasoning\": \"Am început cu un scor de 5/10, dar am adăugat 3 puncte pentru că articolul prezintă informații verificabile despre viața și familia Yoko Ono. Totuși, am scăzut scorul cu 0,5 puncte pentru că articolul nu oferă dovezi concrete pentru a susține legătura de rudenie cu Pușkin.\",
    \"score_breakdown\": \"pornind de la 5/10, +3 pentru inormații despre viața și familia Yoko Ono\",
    \"summary\": \"Articolul prezintă informații despre viața și familia Yoko Ono, dar nu reușește să stabilească o legătură clară între ea și Pușkin.\",
    \"verdict_explanation\": \"Articolul prezintă informații verificabile despre viața și familia Yoko Ono, dar nu oferă dovezi concrete pentru a susține legătura de rudenie cu Pușkin. Prin urmare, verdictul final este PARȚIAL ADEVĂRAT.\",
    \"verification\": {
        \"fake_reasons\": [
            \"Lipsa de dovezi concrete pentru a susține legătura de rudenie între Yoko Ono și Pușkin.\"
        ],
        \"is_fake\": false,
        \"real_information\": \"Yoko Ono s-a născut în Tokyo, într-o familie bogată, și a studiat la o școală din Tokyo.\",
        \"verified_sources\": [
            \"Wikipedia\"
        ]
    }
}
```"""
}

FAKE_STREAM_LOGS = [
        {"type": "start", "message": "🚀 Начинаю проверку..."},
        {"type": "progress", "message": "🌐 Загружаю страницу..."},
        {"type": "progress", "message": "✓ Страница загружена, читаю контент... (7884 символов)"},
        {"type": "progress", "message": "🔬 Начинаю анализ содержимого..."},
        {"type": "progress", "message": "📄 Читаю текст... 7884 символов"},
        {"type": "progress", "message": "🔍 Ищу факты по теме в интернете..."},
        {"type": "progress", "message": "✓ Нашёл дополнительный контекст из сети"},
        {"type": "progress", "message": "🧠 Анализирую текст на манипуляции и дезинформацию... (12656 симв.)"},
        {"type": "progress", "message": "⏳ Проверяю источники, логику и факты..."},
        {"type": "progress", "message": "📊 Обрабатываю результат..."},
        {"type": "progress", "message": "📊 Использовано токенов: 6377 (запрос: 5549, ответ: 828)"},
        {"type": "progress", "message": "📊 Достоверность: 8/10 · манипуляций: 1 · логических ошибок: 1"},
        {"type": "progress", "message": "🟢 Контент выглядит достоверно"},
        {"type": "done", "message": "✅ Готово!"}
]

# Функция для загрузки статей из JSON
def load_articles():
    with open('articles.json', 'r', encoding='utf-8') as f:
        return json.load(f)

# Загрузка данных статей из JSON файла
articles_data = load_articles()

# --- API endpoint for analyze (stub) ---
@app.post("/api/analyze")
async def analyze_api(request: Request):
    data = await request.json()
    url = data.get("url")
    # Здесь будет логика обращения к AI/ML API
    # Пока возвращаем фиктивный json
    return JSONResponse({"result": "ok", "url": url, "verdict": "likely true", "score": 0.87})

# --- API endpoint for analyze (stream proxy) ---
@app.get("/api/analyze/stream")
async def analyze_stream(url: str):
    if Config.USE_FAKE_API_DATA:
        async def fake_event_generator():
            for log in FAKE_STREAM_LOGS:
                yield f"event: {log['type']}\n"
                yield f"data: {log['message']}\n\n"
            yield "event: result\n"
            yield f"data: {json.dumps(FAKE_API_RESULT)}\n\n"
            yield "event: done\n"
            yield "data: ✅ Проверка завершена!\n\n"

        return StreamingResponse(fake_event_generator(), media_type="text/event-stream")

    async def event_generator():
        payload = {"url": url}
        try:
            async with httpx.AsyncClient(timeout=None) as client:
                async with client.stream("POST", ANALYZE_STREAM_URL, json=payload) as resp:
                    async for line in resp.aiter_lines():
                        if line == "":
                            yield "\n"
                            continue
                        if line.startswith("event:") or line.startswith("data:"):
                            yield f"{line}\n"
                            continue
                        yield f"data: {line}\n"
        except Exception as exc:
            error_payload = json.dumps({"error": "stream_failed", "details": str(exc)})
            yield "event: error\n"
            yield f"data: {error_payload}\n\n"

    return StreamingResponse(event_generator(), media_type="text/event-stream")

# --- API endpoint for history stats (proxy) ---
@app.get("/api/history/stats")
async def history_stats():
    """Прокси-эндпоинт для получения статистики истории запросов"""
    try:
        async with httpx.AsyncClient(timeout=30.0) as client:
            response = await client.get(
                ADMIN_STATS_URL,
                headers={"X-Admin-Token": ADMIN_TOKEN}
            )
            response.raise_for_status()
            return JSONResponse(response.json())
    except httpx.HTTPStatusError as exc:
        return JSONResponse(
            {"error": "api_error", "status": exc.response.status_code},
            status_code=exc.response.status_code
        )
    except Exception as exc:
        return JSONResponse(
            {"error": "connection_failed", "details": str(exc)},
            status_code=500
        )

# --- Show API response page ---
@app.get("/api-response", response_class=HTMLResponse)
async def api_response(request: Request, url: str = None):
    lang = get_lang_from_request(request)
    if Config.USE_FAKE_API_DATA:
        api_result = json.loads(json.dumps(FAKE_API_RESULT))
        return templates.TemplateResponse(
            "api-templates/api-response.html",
            {
                "request": request,
                "api_result": api_result,
                "fake_logs": FAKE_STREAM_LOGS,
                "use_fake": True,
                "lang": lang,
                "i18n": TRANSLATIONS.get(lang, {})
            }
        )
    # Реальный режим — нужен URL для анализа
    api_result = {"url": url} if url else None
    return templates.TemplateResponse(
        "api-templates/api-response.html",
        {
            "request": request,
            "api_result": api_result,
            "use_fake": False,
            "lang": lang,
            "i18n": TRANSLATIONS.get(lang, {})
        }
    )
# --- API endpoint for setting language ---
@app.get("/set-lang/{lang}")
async def set_language(lang: str, request: Request):
    """Установить язык и перенаправить на предыдущую страницу"""
    if lang not in SUPPORTED_LANGUAGES:
        lang = DEFAULT_LANGUAGE
    
    referer = request.headers.get('referer', '/')
    response = HTMLResponse(content="", status_code=302)
    response.headers['Location'] = referer
    response.set_cookie(key='lang', value=lang, max_age=31536000)  # 1 год
    return response

# Главная страница GET
@app.get("/", response_class=HTMLResponse)
async def home(request: Request):
    """Главная страница с элементом для замены через Jinja"""
    lang = get_lang_from_request(request)
    return templates.TemplateResponse("mainpage.html", {
        "request": request, 
        "api_post": False,
        "lang": lang,
        "i18n": TRANSLATIONS.get(lang, {})
    })

# Главная страница POST (анализ)
@app.post("/", response_class=HTMLResponse)
async def home_post(request: Request):
    form = await request.form()
    url = form.get("url")
    lang = get_lang_from_request(request)
    
    if Config.USE_FAKE_API_DATA:
        # Используем фейковые данные
        api_result = json.loads(json.dumps(FAKE_API_RESULT))
        api_result["url"] = url  # Подставляем URL из формы
        return templates.TemplateResponse(
            "mainpage.html",
            {
                "request": request,
                "api_post": True,
                "api_result": api_result,
                "fake_logs": FAKE_STREAM_LOGS,
                "use_fake": True,
                "lang": lang,
                "i18n": TRANSLATIONS.get(lang, {})
            }
        )
    
    # Реальный запрос — передаём только URL, данные придут через streaming
    real_result = {"result": "pending", "url": url}
    return templates.TemplateResponse(
        "mainpage.html",
        {
            "request": request,
            "api_post": True,
            "api_result": real_result,
            "use_fake": False,
            "lang": lang,
            "i18n": TRANSLATIONS.get(lang, {})
        }
    )


@app.get("/library", response_class=HTMLResponse)
async def library(request: Request):
    """Страница библиотеки со статьями"""
    lang = get_lang_from_request(request)
    return templates.TemplateResponse("library.html", {
        "request": request,
        "articles": articles_data,
        "lang": lang,
        "i18n": TRANSLATIONS.get(lang, {})
    })


@app.get("/article/{article_id}", response_class=HTMLResponse)
async def get_article(article_id: int, request: Request):
    """Получить информацию о конкретной статье"""
    lang = get_lang_from_request(request)
    article = next((a for a in articles_data if a["id"] == article_id), None)
    if article:
        return templates.TemplateResponse("article-fragment.html", {
            "request": request,
            "article": article,
            "lang": lang,
            "i18n": TRANSLATIONS.get(lang, {})
        })
    return templates.TemplateResponse("404.html", {
        "request": request,
        "lang": lang,
        "i18n": TRANSLATIONS.get(lang, {})
    })


@app.get("/about", response_class=HTMLResponse)
async def about(request: Request):
    """Страница About Us"""
    lang = get_lang_from_request(request)
    return templates.TemplateResponse("about.html", {
        "request": request,
        "lang": lang,
        "i18n": TRANSLATIONS.get(lang, {})
    })


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="127.0.0.1", port=8000)
