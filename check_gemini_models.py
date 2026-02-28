#!/usr/bin/env python3
"""
Проверяет какие модели Gemini доступны для вашего API ключа.
Запуск: python check_gemini_models.py
"""

import os
import json
import urllib.request
import urllib.error

# ── Ключ ──────────────────────────────────────────────────────────
API_KEY = os.environ.get("GEMINI_API_KEY", "")
if not API_KEY:
    env_path = os.path.join(os.path.dirname(__file__), ".env")
    try:
        with open(env_path, encoding="utf-8", errors="ignore") as f:
            for line in f:
                line = line.strip()
                if line.startswith("GEMINI_API_KEY="):
                    API_KEY = line.split("=", 1)[1].strip().strip('"').strip("'")
                    break
    except FileNotFoundError:
        pass

if not API_KEY:
    print("GEMINI_API_KEY не найден.")
    exit(1)

print(f"Ключ: {API_KEY[:8]}...{API_KEY[-4:]}\n")

BASE = "https://generativelanguage.googleapis.com/v1beta"

# ── Получить список моделей ────────────────────────────────────────
def list_models():
    url = f"{BASE}/models?key={API_KEY}&pageSize=200"
    with urllib.request.urlopen(url, timeout=10) as r:
        data = json.loads(r.read())
        return data.get("models", [])

print("Получаю список моделей...")
models = list_models()
generate_models = [
    m["name"].replace("models/", "")
    for m in models
    if "generateContent" in m.get("supportedGenerationMethods", [])
]
print(f"Найдено с generateContent: {len(generate_models)}\n")

# ── Тест текстового запроса ────────────────────────────────────────
def test_text(model):
    url = f"{BASE}/models/{model}:generateContent?key={API_KEY}"
    body = json.dumps({
        "contents": [{"parts": [{"text": "Reply with one word: OK"}]}],
        "generationConfig": {"maxOutputTokens": 5}
    }).encode()
    req = urllib.request.Request(url, data=body,
                                  headers={"Content-Type": "application/json"},
                                  method="POST")
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            data = json.loads(r.read())
            text = data["candidates"][0]["content"]["parts"][0]["text"]
            return "OK", text.strip()[:30]
    except urllib.error.HTTPError as e:
        body_str = e.read().decode(errors="ignore")
        try:
            msg = json.loads(body_str)["error"]["message"][:70]
        except Exception:
            msg = body_str[:70]
        return f"{e.code}", msg
    except Exception as ex:
        return "ERR", str(ex)[:70]

# ── Тест file_data ─────────────────────────────────────────────────
def test_filedata(model):
    url = f"{BASE}/models/{model}:generateContent?key={API_KEY}"
    body = json.dumps({
        "contents": [{
            "parts": [
                {"file_data": {"mime_type": "video/mp4",
                               "file_uri": f"{BASE}/files/fake_probe_xyz"}},
                {"text": "Describe."}
            ]
        }],
        "generationConfig": {"maxOutputTokens": 5}
    }).encode()
    req = urllib.request.Request(url, data=body,
                                  headers={"Content-Type": "application/json"},
                                  method="POST")
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            return "YES"
    except urllib.error.HTTPError as e:
        body_str = e.read().decode(errors="ignore")
        if "Unknown name" in body_str or "Cannot find field" in body_str:
            return "NO (no file_data)"
        if e.code == 429:
            return "YES (quota, but field ok)"
        if e.code == 404 and ("fake_probe" in body_str or "not found" in body_str.lower() and "model" not in body_str.lower()):
            return "YES (file not found, field ok)"
        if e.code == 404:
            return "NO (model 404)"
        if e.code == 400:
            try:
                msg = json.loads(body_str)["error"]["message"][:90]
            except Exception:
                msg = body_str[:90]
            if "Unknown name" in msg or "Cannot find field" in msg:
                return "NO (no file_data)"
            return f"YES? 400: {msg}"
        return f"? {e.code}"
    except Exception as ex:
        return f"ERR {str(ex)[:40]}"

# ── Тест всех моделей ──────────────────────────────────────────────
print(f"{'Модель':<45} {'Текст':<8} {'file_data':<28} Ответ")
print("─" * 110)

text_ok  = []
video_ok = []

for model in generate_models:
    status, reply = test_text(model)
    if status == "OK":
        fd = test_filedata(model)
        text_ok.append(model)
        if "YES" in fd:
            video_ok.append(model)
    else:
        fd = "—"
        # Для 429 проверим file_data (модель есть, просто квота)
        if status == "429":
            fd = test_filedata(model)
            if "YES" in fd:
                video_ok.append(f"{model} (need quota)")

    mark = "✅" if status == "OK" else ("⚠️ " if status == "429" else "❌")
    print(f"{model:<45} {mark} {status:<5}  {fd:<28} {reply if status == 'OK' else ''}")

# ── Итог ──────────────────────────────────────────────────────────
print("\n" + "═" * 110)
print("ИТОГ\n")

if text_ok:
    print("Работают для текстовых запросов:")
    for m in text_ok:
        print(f"  • {m}")
else:
    print("Ни одна модель не ответила на текстовый запрос")
    print("→ Квота исчерпана или нужен новый ключ на https://aistudio.google.com")

if video_ok:
    print("\nПоддерживают file_data (видеоанализ):")
    for m in video_ok:
        print(f"  • {m}")
    # Рекомендация — первая без "(need quota)"
    best = next((m for m in video_ok if "need quota" not in m), video_ok[0])
    print(f"\n>>> Использовать в боте: '{best}'")
else:
    print("\nНи одна модель не поддерживает file_data")
