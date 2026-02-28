import requests
import json

# Analyze URL with SSE streaming
url = "https://apich.sinkdev.dev/api/analyze/stream"
data = {"url": "https://provereno.media/blog/2026/02/25/pravda-li-chto-yoko-ono-rodstvennitsa-pushkina/"}

response = requests.post(url, json=data, stream=True)

for line in response.iter_lines():
    if not line:
        continue

    decoded_line = line.decode('utf-8')
    if decoded_line.startswith('event:'):
        event = decoded_line[7:].strip()
        print(f"Event: {event}")
        continue

    if not decoded_line.startswith('data:'):
        continue

    payload = decoded_line[5:].strip()
    if not payload:
        continue

    if payload == "[DONE]":
        break

    try:
        data = json.loads(payload)
    except json.JSONDecodeError:
        print(f"{payload}")
        continue

    print(f"Data: {data}")