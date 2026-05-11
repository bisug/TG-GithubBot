package config

import (
	"encoding/hex"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken       string
	TelegramWebhookURL  string
	MongoDBURI          string
	DatabaseName        string
	GitHubWebhookSecret string
	GitHubClientID      string
	GitHubClientSecret  string
	Port                string
	EncryptionKey       string
}

func Load() *Config {
	_ = godotenv.Load()

	required := []string{
		"TELEGRAM_TOKEN",
		"TELEGRAM_WEBHOOK_URL",
		"MONGODB_URI",
		"GITHUB_CLIENT_ID",
		"GITHUB_CLIENT_SECRET",
		"GITHUB_WEBHOOK_SECRET",
		"ENCRYPTION_KEY",
	}

	var missing []string
	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		log.Fatalf("Missing required environment variables: %s", strings.Join(missing, ", "))
	}

	if err := validateEncryptionKey(os.Getenv("ENCRYPTION_KEY")); err != nil {
		log.Fatalf("Invalid ENCRYPTION_KEY: %v", err)
	}

	return &Config{
		TelegramToken:       os.Getenv("TELEGRAM_TOKEN"),
		TelegramWebhookURL:  strings.TrimRight(os.Getenv("TELEGRAM_WEBHOOK_URL"), "/"),
		MongoDBURI:          os.Getenv("MONGODB_URI"),
		DatabaseName:        getEnv("DATABASE_NAME", "github_bot"),
		GitHubWebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		GitHubClientID:      os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret:  os.Getenv("GITHUB_CLIENT_SECRET"),
		Port:                getEnv("PORT", "8080"),
		EncryptionKey:       os.Getenv("ENCRYPTION_KEY"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func validateEncryptionKey(key string) error {
	if len(key) == 64 {
		if _, err := hex.DecodeString(key); err != nil {
			return err
		}
		return nil
	}

	if len(key) == 32 || len(key) == 24 || len(key) == 16 {
		return nil
	}

	return errInvalidEncryptionKeyLength{}
}

type errInvalidEncryptionKeyLength struct{}

func (errInvalidEncryptionKeyLength) Error() string {
	return "must be 16, 24, or 32 raw characters, or 64 hex characters from 32 random bytes"
}
