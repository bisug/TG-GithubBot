package github

import (
	"context"
	"fmt"

	"github-webhook/internal/db"
	"github-webhook/internal/utils"

	"github.com/google/go-github/v85/github"
)

// GetClientForUser retrieves the user's OAuth token from the database, decrypts it,
// and returns an authenticated GitHub client.
func GetClientForUser(ctx context.Context, database *db.DB, factory *ClientFactory, userID int64, encryptionKey string) (*github.Client, error) {
	user, err := database.GetUserByTelegramID(ctx, userID)
	if err != nil || user.EncryptedOAuthToken == "" {
		return nil, fmt.Errorf("unauthorized")
	}

	token, err := utils.Decrypt(user.EncryptedOAuthToken, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return factory.GetUserClient(ctx, token), nil
}
