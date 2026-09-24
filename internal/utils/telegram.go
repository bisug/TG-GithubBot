package utils

import (
	"log/slog"
	"time"

	"github-webhook/internal/cache"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// adminCacheTTL bounds how long a cached admin check is trusted. Telegram
// admin changes are rare; a short TTL keeps permission revocations effective
// while removing one GetChatMember API call per command/callback tap.
const adminCacheTTL = 2 * time.Minute

// adminKey pairs the chat and user IDs; a struct key cannot collide the way
// packed int64 keys could for Telegram's wide (often negative) IDs.
type adminKey struct {
	chatID, userID int64
}

var adminCache = cache.New[adminKey, bool]()

// CleanupAdminCache sweeps expired admin entries so the cache does not grow
// unbounded for chats that never interact with the bot again.
func CleanupAdminCache() {
	adminCache.Cleanup()
}

// InvalidateAdmin removes the cached permission for one chat/user pair so the
// next check contacts Telegram.
func InvalidateAdmin(chatID, userID int64) {
	adminCache.Delete(adminKey{chatID, userID})
}

// IsAdmin reports whether userID is an administrator or the creator of chatID.
// Results are cached briefly to avoid one Telegram API call per interaction.
func IsAdmin(b *gotgbot.Bot, chatID int64, userID int64) bool {
	key := adminKey{chatID, userID}
	if cached, ok := adminCache.Get(key); ok {
		return cached
	}

	member, err := b.GetChatMember(chatID, userID, nil)
	if err != nil {
		// Cache the negative result briefly too: a failing GetChatMember would
		// otherwise be retried on every tap in a busy group.
		slog.Debug("GetChatMember failed", "chat", chatID, "user", userID, "error", err)
		adminCache.Set(key, false, adminCacheTTL)
		return false
	}

	status := member.GetStatus()
	isAdmin := status == "administrator" || status == "creator"
	adminCache.Set(key, isAdmin, adminCacheTTL)
	return isAdmin
}
