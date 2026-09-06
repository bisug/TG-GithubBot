package config

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken          string
	TelegramWebhookURL     string
	MongoDBURI             string
	DatabaseName           string
	GitHubWebhookSecret    string
	GitHubClientID         string
	GitHubClientSecret     string
	Port                   string
	EncryptionKey          string
	UsePolling             bool
	GitHubAppID            int64  // 0 = GitHub App mode disabled
	GitHubAppPrivateKey    string // path to the app's PEM key
	GitHubAppWebhookSecret string // falls back to GitHubWebhookSecret
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

	// GitHub App mode is optional: set GITHUB_APP_ID + GITHUB_APP_PRIVATE_KEY
	// to receive app-level webhook events on /app-webhook/<token>.
	githubAppID := int64(0)
	if raw := strings.TrimSpace(os.Getenv("GITHUB_APP_ID")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			slog.Error("Invalid GITHUB_APP_ID (must be numeric)", "error", err)
			os.Exit(1)
		}
		githubAppID = id
	}

	githubAppWebhookSecret := strings.TrimSpace(os.Getenv("GITHUB_APP_WEBHOOK_SECRET"))
	if githubAppWebhookSecret == "" {
		githubAppWebhookSecret = strings.TrimSpace(os.Getenv("GITHUB_WEBHOOK_SECRET"))
	}

	return &Config{
		TelegramToken:          strings.TrimSpace(os.Getenv("TELEGRAM_TOKEN")),
		TelegramWebhookURL:     strings.TrimRight(strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_URL")), "/"),
		MongoDBURI:             strings.TrimSpace(os.Getenv("MONGODB_URI")),
		DatabaseName:           getEnv("DATABASE_NAME", "github_bot"),
		GitHubWebhookSecret:    strings.TrimSpace(os.Getenv("GITHUB_WEBHOOK_SECRET")),
		GitHubClientID:         strings.TrimSpace(os.Getenv("GITHUB_CLIENT_ID")),
		GitHubClientSecret:     strings.TrimSpace(os.Getenv("GITHUB_CLIENT_SECRET")),
		Port:                   getEnv("PORT", "8080"),
		EncryptionKey:          encryptionKey,
		UsePolling:             usePolling,
		GitHubAppID:            githubAppID,
		GitHubAppPrivateKey:    strings.TrimSpace(os.Getenv("GITHUB_APP_PRIVATE_KEY")),
		GitHubAppWebhookSecret: githubAppWebhookSecret,
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return strings.TrimSpace(value)
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
