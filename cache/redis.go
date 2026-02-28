package cache

import (
	"context"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
)

var RDB *redis.Client
var ctx = context.Background()

func InitRedis(url string) {
	if url == "" {
		log.Println("⚠️ REDIS_URL не установлен, работа без кэша")
		return
	}

	RDB = redis.NewClient(&redis.Options{
		Addr: url,
	})

	_, err := RDB.Ping(ctx).Result()
	if err != nil {
		log.Printf("⚠️ Redis недоступен: %v", err)
		RDB = nil
		return
	}

	log.Println("✓ Подключение к Redis установлено")
}

func Get(key string) (string, error) {
	if RDB == nil {
		return "", redis.Nil
	}
	return RDB.Get(ctx, key).Result()
}

func Set(key string, value string, expiration time.Duration) error {
	if RDB == nil {
		return nil
	}
	return RDB.Set(ctx, key, value, expiration).Err()
}
