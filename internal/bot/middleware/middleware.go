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

func TrackUserAndChat(database *db.DB) func(b *gotgbot.Bot, ctx *ext.Context) error {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		if ctx.EffectiveChat != nil {
			chatType := ctx.EffectiveChat.Type
			dbChat := &models.Chat{
				ID:       ctx.EffectiveChat.Id,
				ChatType: chatType,
				Title:    ctx.EffectiveChat.Title,
			}
			if dbChat.Title == "" {
				dbChat.Title = ctx.EffectiveChat.Username
			}

			if _, seen := chatUpsertSeen.Get(ctx.EffectiveChat.Id); seen {
				return nil
			}
			chatUpsertSeen.Set(ctx.EffectiveChat.Id, struct{}{}, 10*time.Minute)

			go func() {
				_ = database.UpsertChat(context.Background(), dbChat)
			}()
		}
		return nil
	}
}
