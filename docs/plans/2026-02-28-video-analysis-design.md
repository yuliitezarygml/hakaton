# Video Analysis Feature — Design

**Date:** 2026-02-28
**Feature:** Telegram bot video analysis for disinformation detection

## Problem

The existing bot accepts text and URLs. Users increasingly share short video clips (~10 seconds) containing spoken disinformation, propaganda, or manipulated visual content. The bot needs to analyze these videos.

## Approach

**Gemini Files API + existing analysis pipeline.**

1. User sends a video to the bot (up to ~10 seconds, up to 50 MB)
2. Bot downloads the video file from Telegram
3. Bot uploads it to Google Gemini Files API (free, no ffmpeg needed)
4. Gemini 2.0 Flash transcribes the speech and describes visual content
5. Combined transcript + visual description is passed to `/api/analyze/stream`
6. Result is displayed in the same format as text analysis (score 0–10, manipulations, why)

## Why Gemini Files API

- **Free:** 1500 requests/day, 50 req/min on Google AI Studio tier
- **No ffmpeg needed:** Gemini handles video natively (mp4, mov, webm, etc.)
- **Combined analysis:** One call transcribes speech AND describes frames
- **API key:** Free at aistudio.google.com
- **Video limit:** up to 2 GB per file, stored 48 hours

## Architecture

```
User sends video
     ↓
handleMessage() — detects msg.Video or msg.Animation
     ↓
downloadTelegramFile() — download via Telegram getFile
     ↓
uploadToGemini() — POST to files.googleapis.com
     ↓
analyzeVideoWithGemini() — send prompt:
  "Transcribe speech. Describe what is shown visually."
     ↓
combined text (transcript + visual description)
     ↓
StreamAnalyze() → /api/analyze/stream
     ↓
Result: credibility score, manipulations, logical issues
```

## Files Changed

| File | Change |
|------|--------|
| `telegram-bot/gemini.go` | New — Gemini Files API upload + video analysis |
| `telegram-bot/main.go` | Add `msg.Video` / `msg.Animation` handling in `handleMessage` |
| `.env` / `.env.example` | Add `GEMINI_API_KEY` |
| `telegram-bot/go.mod` | Add `google.golang.org/genai` or use raw HTTP |

## Gemini Prompt Design

```
Analyze this video and return two sections:

SPEECH TRANSCRIPT:
[Transcribe all spoken words verbatim. If no speech, write "No speech detected."]

VISUAL DESCRIPTION:
[Describe what is visually shown: setting, people, text on screen, graphics, maps, emotional tone.]
```

The output is concatenated and sent as `{"text": "..."}` to the analyzer backend.

## User-facing Flow

```
🎬 Видео получено (10 сек)...
📤 Загружаю в Gemini для расшифровки...
📝 Речь: "В Украине запустили биолабораторию..."
🖼 На видео: человек в студии, карта Украины
⏳ Анализирую текст...

🔴 Балл достоверности: 2/10
Вердикт: Дезинформация
• Манипуляция: утверждение без источников
• Почему: заявление опровергнуто официальными данными
```

## Error Cases

- Video too large (>50 MB): inform user with file size limit message
- Gemini returns empty transcript (silent/music video): proceed with visual-only description
- Gemini upload fails: fallback error message
- No `GEMINI_API_KEY`: disable video feature, send helpful error

## Environment Variables

```
GEMINI_API_KEY=your_key_from_aistudio.google.com
```
