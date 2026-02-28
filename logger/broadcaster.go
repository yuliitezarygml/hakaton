package logger

import (
	"io"
	"os"
	"sync"
)

// Broadcaster — это io.Writer, который дублирует вывод в консоль и в каналы подписчиков
type Broadcaster struct {
	mu          sync.Mutex
	subscribers map[chan string]bool
}

var Instance = &Broadcaster{
	subscribers: make(map[chan string]bool),
}

func (b *Broadcaster) Write(p []byte) (n int, err error) {
	msg := string(p)

	// Пишем в консоль
	os.Stdout.Write(p)

	// Рассылаем всем активным WebSocket подключениям
	b.mu.Lock()
	for ch := range b.subscribers {
		// Используем select, чтобы не блокироваться, если клиент медленно читает
		select {
		case ch <- msg:
		default:
		}
	}
	b.mu.Unlock()

	return len(p), nil
}

// Subscribe создает новый канал для получения логов
func (b *Broadcaster) Subscribe() chan string {
	ch := make(chan string, 100)
	b.mu.Lock()
	b.subscribers[ch] = true
	b.mu.Unlock()
	return ch
}

// Unsubscribe удаляет канал из рассылки
func (b *Broadcaster) Unsubscribe(ch chan string) {
	b.mu.Lock()
	delete(b.subscribers, ch)
	b.mu.Unlock()
	close(ch)
}

// MultiWriter объединяет broadcaster и стандартный логгер
func GetWriter() io.Writer {
	return Instance
}
