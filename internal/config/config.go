package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	DatabaseURL, TelegramToken, TelegramChatID, TelegramProxy string
	AIBaseURL, AIAPIKey, AIModel                              string
	Timezone, DigestTime, CollectInterval                     string
	MaxArticles                                               int
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		TelegramToken:   os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:  os.Getenv("TELEGRAM_CHAT_ID"),
		TelegramProxy:   os.Getenv("TELEGRAM_PROXY"),
		AIBaseURL:       env("AI_BASE_URL", "https://api.avalai.ir/v1"),
		AIAPIKey:        os.Getenv("AI_API_KEY"),
		AIModel:         env("AI_MODEL", "gpt-4o-mini"),
		Timezone:        env("APP_TIMEZONE", "Asia/Tehran"),
		DigestTime:      env("DIGEST_TIME", "08:00"),
		CollectInterval: env("COLLECT_INTERVAL", "30m"),
		MaxArticles:     16,
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if c.TelegramToken == "" || c.TelegramChatID == "" {
		return c, fmt.Errorf("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are required")
	}
	if c.AIAPIKey == "" {
		return c, fmt.Errorf("AI_API_KEY is required")
	}
	if _, err := time.Parse("15:04", c.DigestTime); err != nil {
		return c, fmt.Errorf("invalid DIGEST_TIME: %w", err)
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return c, fmt.Errorf("invalid APP_TIMEZONE: %w", err)
	}
	return c, nil
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
