package utils

import (
	"encoding/hex"
	"testing"
)

// TestAdminKeyDistinct verifies that distinct (chat, user) pairs map to
// distinct cache keys. The old chatID<<32|userID packing collided for the wide
// 64-bit (often negative) IDs Telegram actually uses; the struct key cannot
// collide by construction, so this pins the property against regressions to a
// packed/hashed key.
func TestAdminKeyDistinct(t *testing.T) {
	// Realistic Telegram IDs: group chats are negative and can exceed 32 bits.
	pairs := []adminKey{
		{chatID: -100123456789, userID: 123456789},
		{chatID: -100123456789, userID: -9876543212345},
		{chatID: -100987654321, userID: 123456789},
		{chatID: 123456789, userID: -100123456789},
		{chatID: 8539254782, userID: 2147483648},
		{chatID: 2147483648, userID: 8539254782},
		{chatID: 42, userID: 7},
		{chatID: 6, userID: 49},
	}
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[i] == pairs[j] {
				t.Fatalf("distinct pairs compare equal: %v vs %v", pairs[i], pairs[j])
			}
		}
	}
	if (adminKey{chatID: -100123456789, userID: 123456789} != adminKey{chatID: -100123456789, userID: 123456789}) {
		t.Fatal("adminKey comparison must be deterministic")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	tests := []struct {
		name      string
		plainText string
		key       string
		wantErr   bool
	}{
		{
			name:      "Valid 32-byte key",
			plainText: "Hello World",
			key:       "12345678901234567890123456789012",
			wantErr:   false,
		},
		{
			name:      "Valid 64-char hex key",
			plainText: "Hello World",
			key:       hex.EncodeToString([]byte("12345678901234567890123456789012")),
			wantErr:   false,
		},
		{
			name:      "Invalid key length",
			plainText: "Hello World",
			key:       "shortkey",
			wantErr:   true,
		},
		{
			name:      "Empty text",
			plainText: "",
			key:       "12345678901234567890123456789012",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := Encrypt(tt.plainText, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Encrypt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			decrypted, err := Decrypt(encrypted, tt.key)
			if err != nil {
				t.Errorf("Decrypt() error = %v", err)
				return
			}

			if decrypted != tt.plainText {
				t.Errorf("Decrypt() = %v, want %v", decrypted, tt.plainText)
			}
		})
	}
}

func TestEncoding(t *testing.T) {
	key := "12345678901234567890123456789012"
	plainText := "test text"

	// Run multiple times to account for random nonce
	for i := 0; i < 100; i++ {
		encrypted, err := Encrypt(plainText, key)
		if err != nil {
			t.Fatalf("Encrypt() error = %v", err)
		}

		for _, char := range encrypted {
			if char == '/' || char == '+' {
				t.Errorf("Iteration %d: Encrypted text contains non-URL-safe character: %c in %s", i, char, encrypted)
			}
		}

		decrypted, err := Decrypt(encrypted, key)
		if err != nil {
			t.Fatalf("Iteration %d: Decrypt() error = %v", i, err)
		}
		if decrypted != plainText {
			t.Errorf("Iteration %d: Decrypted = %v, want %v", i, decrypted, plainText)
		}
	}
}

func TestBackwardCompatibility(t *testing.T) {
	key := "12345678901234567890123456789012"
	plainText := "backward compatibility test"

	encryptedStd, err := EncryptStd(plainText, key)
	if err != nil {
		t.Fatalf("EncryptStd() error = %v", err)
	}

	decrypted, err := Decrypt(encryptedStd, key)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if decrypted != plainText {
		t.Errorf("Decrypt() = %v, want %v", decrypted, plainText)
	}
}

func TestDecryptErrors(t *testing.T) {
	key := "12345678901234567890123456789012"

	t.Run("Invalid base64", func(t *testing.T) {
		_, err := Decrypt("invalid-base64", key)
		if err == nil {
			t.Error("Decrypt() expected error for invalid base64")
		}
	})

	t.Run("Short ciphertext", func(t *testing.T) {
		_, err := Decrypt("SHORT", key)
		if err == nil {
			t.Error("Decrypt() expected error for short ciphertext")
		}
	})
}
