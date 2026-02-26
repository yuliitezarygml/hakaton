# API Чата - Документация

## Endpoint: `/api/chat`

Метод для общения с AI-помощником на основе результатов анализа новостей.

### Метод: POST

### URL: `http://localhost:8080/api/chat`

### CORS
API поддерживает CORS и может быть вызван с любого домена.

### Заголовки запроса
```
Content-Type: application/json
```

### Тело запроса

```json
{
  "message": "Почему эта новость недостоверна?",
  "analysis_context": {
    "summary": "Краткое резюме анализа...",
    "credibility_score": 3,
    "manipulations": ["Манипуляция 1", "Манипуляция 2"],
    "logical_issues": ["Ошибка 1", "Ошибка 2"],
    "reasoning": "Обоснование оценки...",
    "fact_check": {
      "verifiable_facts": ["Факт 1"],
      "opinions_as_facts": ["Мнение 1"],
      "missing_evidence": ["Утверждение без доказательств"]
    },
    "verification": {
      "is_fake": true,
      "fake_reasons": ["Причина 1", "Причина 2"],
      "real_information": "Настоящая информация..."
    }
  }
}
```

### Параметры

- `message` (string, обязательный) - Вопрос пользователя к AI
- `analysis_context` (object, опциональный) - Результаты анализа новости для контекста

### Ответ

```json
{
  "response": "Эта новость недостоверна по следующим причинам...",
  "usage": {
    "prompt_tokens": 1234,
    "completion_tokens": 567,
    "total_tokens": 1801
  }
}
```

### Поля ответа

- `response` (string) - Ответ AI на вопрос пользователя
- `usage` (object) - Информация об использованных токенах
  - `prompt_tokens` (int) - Токены в запросе
  - `completion_tokens` (int) - Токены в ответе
  - `total_tokens` (int) - Всего токенов

## Примеры использования

### JavaScript (Fetch API)

```javascript
const response = await fetch('http://localhost:8080/api/chat', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    message: 'Какие манипуляции использованы в этой новости?',
    analysis_context: analysisResult // результат от /api/analyze
  })
});

const data = await response.json();
console.log('Ответ:', data.response);
console.log('Использовано токенов:', data.usage.total_tokens);
```

### cURL

```bash
curl -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Почему эта новость недостоверна?",
    "analysis_context": null
  }'
```

### Без контекста анализа

Можно задавать общие вопросы без контекста:

```javascript
const response = await fetch('http://localhost:8080/api/chat', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    message: 'Как распознать фейковые новости?'
  })
});
```

## Системный промпт

AI настроен как помощник по анализу новостей с следующими характеристиками:

- Отвечает только на русском языке
- Использует результаты анализа для обоснования ответов
- Объясняет манипуляции и логические ошибки простым языком
- Честно признается, если не знает ответа
- Не придумывает информацию

## Коды ошибок

- `400 Bad Request` - Неверный формат запроса или пустое сообщение
- `405 Method Not Allowed` - Использован неподдерживаемый HTTP метод
- `500 Internal Server Error` - Ошибка при обработке запроса

## Примечания

1. Поле `analysis_context` может быть `null` или отсутствовать для общих вопросов
2. Рекомендуется передавать полный контекст анализа для более точных ответов
3. API автоматически форматирует контекст для AI
4. Токены учитываются для всех запросов (Groq и OpenRouter)
