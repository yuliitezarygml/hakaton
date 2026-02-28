# Documentation: Streaming API (`/api/analyze/stream`)

Этот эндпоинт позволяет получать результаты анализа текста в режиме реального времени (Streaming) с использованием **Server-Sent Events (SSE)**. Это полезно для отображения процесса анализа пользователю по мере его выполнения нейросетью.

## Основная информация

- **URL**: `http://localhost/api/analyze/stream`
- **Метод**: `POST`
- **Content-Type**: `application/json`
- **Response Format**: `text/event-stream`

---

## Формат запроса

Вы можете отправить либо прямую ссылку на статью, либо сырой текст.

### Пример с URL:
```json
{
  "url": "https://example.com/article"
}
```

### Пример с текстом:
```json
{
  "text": "Ваш текст для анализа здесь..."
}
```

---

## Формат ответа (SSE)

Сервер отправляет данные по частям. Каждое сообщение начинается с `data: `.

### Структура потока:

1.  **Промежуточные куски (chunks)**: содержат части текстового ответа от ИИ.
    ```text
    data: {"content": "Анализ "}
    data: {"content": "показывает, "}
    data: {"content": "что..."}
    ```

2.  **Финальный объект**: когда анализ завершен, сервер отправляет полный структурированный JSON с результатами, включая оценку доверия.
    ```text
    data: {"result": {"credibility_score": 8, "verification": {...}, "verdict": "..."}}
    ```

---

## Пример использования (JavaScript/Frontend)

```javascript
async function startStreaming() {
  const response = await fetch('http://localhost/api/analyze/stream', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url: 'https://...' })
  });

  const reader = response.body.getReader();
  const decoder = new TextDecoder();

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;

    const chunk = decoder.decode(value);
    const lines = chunk.split('\n');

    for (const line of lines) {
      if (line.startsWith('data: ')) {
        const jsonStr = line.replace('data: ', '');
        try {
          const data = JSON.parse(jsonStr);
          if (data.content) {
            console.log("Печатаем:", data.content);
          } else if (data.result) {
            console.log("Финальный результат:", data.result);
          }
        } catch (e) {
          // Игнорируем неполные JSON
        }
      }
    }
  }
}
```

## Преимущества стриминга
- **UX**: Пользователь видит, что программа работает, не дожидаясь полного ответа (который может занять 10-20 секунд).
- **Стабильность**: Соединение держится открытым до завершения генерации.
- **Интерактивность**: Можно реализовать эффект "печатающейся машинки" в интерфейсе.
