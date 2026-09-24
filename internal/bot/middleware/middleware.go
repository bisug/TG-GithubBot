package middleware

import (
	"context"
	"time"

	"github-webhook/internal/cache"
	"github-webhook/internal/db"
	"github-webhook/internal/models"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// chatUpsertSeen debounces chat upserts so we write to the DB at most once per chat
// every 10 minutes instead of on every incoming update.
var chatUpsertSeen = cache.New[int64, struct{}]()

// CleanupChatUpsertSeen sweeps expired debounce entries so the cache does not grow
// unbounded for chats that never message the bot again. Called from the main ticker.
func CleanupChatUpsertSeen() {
	chatUpsertSeen.Cleanup()
}

func TrackUserAndChat(database *db.DB) func(b *gotgbot.Bot, ctx *ext.Context) error {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		if ctx.EffectiveChat != nil {
			ctxChatID := ctx.EffectiveChat.Id
			chatType := ctx.EffectiveChat.Type
			dbChat := &models.Chat{
				ID:       ctx.EffectiveChat.Id,
				ChatType: chatType,
				Title:    ctx.EffectiveChat.Title,
			}
			if dbChat.Title == "" {
				dbChat.Title = ctx.EffectiveChat.Username
			}

			if !chatUpsertSeen.AddIfAbsent(ctxChatID, struct{}{}, 10*time.Minute) {
				return nil
			}

			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := database.UpsertChat(ctx, dbChat); err != nil {
					// Allow a later update to retry instead of suppressing chat
					// metadata changes for the full debounce window.
					chatUpsertSeen.Delete(ctxChatID)
				}
			}()
		}
		return nil
	}
}
