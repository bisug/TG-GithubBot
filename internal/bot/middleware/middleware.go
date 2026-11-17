package middleware

import (
	"context"
	"time"

	"github-webhook/internal/db"
	"github-webhook/internal/models"

	"github.com/AshokShau/gotdbot"
)

func TrackChat(database *db.DB) func(c *gotdbot.Client, u *gotdbot.UpdateNewChat) error {
	return func(c *gotdbot.Client, u *gotdbot.UpdateNewChat) error {
		if u.Chat == nil {
			return nil
		}
		chat := u.Chat
		dbChat := &models.Chat{
			ID:       chat.Id,
			ChatType: chat.Type.GetType(),
			Title:    chat.Title,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return database.UpsertChat(ctx, dbChat)
	}
}
