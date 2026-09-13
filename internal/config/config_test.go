package config

import (
	"os"
	"strings"
	"testing"
)

func TestValidateEncryptionKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "16-byte raw key", key: "1234567890123456", wantErr: false},
		{name: "24-byte raw key", key: "123456789012345678901234", wantErr: false},
		{name: "32-byte raw key", key: "12345678901234567890123456789012", wantErr: false},
		{name: "64-char hex key", key: strings.Repeat("a", 64), wantErr: false},
		{name: "64-char invalid hex", key: strings.Repeat("z", 64), wantErr: true},
		{name: "empty key", key: "", wantErr: true},
		{name: "too short key", key: "short", wantErr: true},
		{name: "20-byte key", key: "12345678901234567890", wantErr: true},
		{name: "63-char hex key", key: strings.Repeat("a", 63), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEncryptionKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateEncryptionKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	const testKey = "TEST_GITHUB_BOT_CONFIG_ENV"
	_ = os.Unsetenv(testKey)

	if got := getEnv(testKey, "default_val"); got != "default_val" {
		t.Fatalf("getEnv() unset = %q, want %q", got, "default_val")
	}

	_ = os.Setenv(testKey, "  custom_val  ")
	defer func() { _ = os.Unsetenv(testKey) }()

	if got := getEnv(testKey, "default_val"); got != "custom_val" {
		t.Fatalf("getEnv() set = %q, want %q (trimmed)", got, "custom_val")
	}
}
