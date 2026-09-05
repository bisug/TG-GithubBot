package config

import (
	"encoding/hex"
	"fmt"
	"log/slog"
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
	UsePolling          bool
}

func Load() *Config {
	_ = godotenv.Load()

	usePolling := strings.ToLower(os.Getenv("USE_POLLING")) == "true"

	required := []string{
		"TELEGRAM_TOKEN",
		"TELEGRAM_WEBHOOK_URL",
		"MONGODB_URI",
		"GITHUB_CLIENT_ID",
		"GITHUB_CLIENT_SECRET",
		"ENCRYPTION_KEY",
	}

	var missing []string
	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		slog.Error("Missing required environment variables", "missing", strings.Join(missing, ", "))
		os.Exit(1)
	}

	encryptionKey := strings.TrimSpace(os.Getenv("ENCRYPTION_KEY"))
	if err := validateEncryptionKey(encryptionKey); err != nil {
		slog.Error("Invalid ENCRYPTION_KEY", "error", err)
		os.Exit(1)
	}

	return &Config{
		TelegramToken:       strings.TrimSpace(os.Getenv("TELEGRAM_TOKEN")),
		TelegramWebhookURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_URL")), "/"),
		MongoDBURI:          strings.TrimSpace(os.Getenv("MONGODB_URI")),
		DatabaseName:        getEnv("DATABASE_NAME", "github_bot"),
		GitHubWebhookSecret: strings.TrimSpace(os.Getenv("GITHUB_WEBHOOK_SECRET")),
		GitHubClientID:      strings.TrimSpace(os.Getenv("GITHUB_CLIENT_ID")),
		GitHubClientSecret:  strings.TrimSpace(os.Getenv("GITHUB_CLIENT_SECRET")),
		Port:                getEnv("PORT", "8080"),
		EncryptionKey:       encryptionKey,
		UsePolling:          usePolling,
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

	return fmt.Errorf("must be 16, 24, or 32 raw characters, or 64 hex characters from 32 random bytes; got length %d after trimming spaces", len(key))
}
