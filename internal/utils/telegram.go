package utils

import (
	"log/slog"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// adminCacheTTL bounds how long a cached admin check is trusted. Telegram
// admin changes are rare; a short TTL keeps permission revocations effective
// while removing one GetChatMember API call per command/callback tap.
const adminCacheTTL = 2 * time.Minute

type adminResult struct {
	isAdmin bool
	expires time.Time
}

var (
	adminMu    sync.RWMutex
	adminCache = map[int64]adminResult{} // key: chatID<<32 | userID (both fit in int64)
)

// adminCacheKey packs chat and user IDs into a single int64 map key.
func adminCacheKey(chatID, userID int64) int64 {
	return chatID<<32 | userID
}

// CleanupAdminCache sweeps expired admin entries so the map does not grow
// unbounded for chats that never interact with the bot again.
func CleanupAdminCache() {
	now := time.Now()
	adminMu.Lock()
	for k, v := range adminCache {
		if now.After(v.expires) {
			delete(adminCache, k)
		}
	}
	adminMu.Unlock()
}

// IsAdmin reports whether userID is an administrator or the creator of chatID.
// Results are cached briefly to avoid one Telegram API call per interaction.
func IsAdmin(b *gotgbot.Bot, chatID int64, userID int64) bool {
	key := adminCacheKey(chatID, userID)

	adminMu.RLock()
	cached, ok := adminCache[key]
	adminMu.RUnlock()
	if ok && time.Now().Before(cached.expires) {
		return cached.isAdmin
	}

	member, err := b.GetChatMember(chatID, userID, nil)
	if err != nil {
		// Cache the negative result briefly too: a failing GetChatMember would
		// otherwise be retried on every tap in a busy group.
		slog.Debug("GetChatMember failed", "chat", chatID, "user", userID, "error", err)
		adminMu.Lock()
		adminCache[key] = adminResult{isAdmin: false, expires: time.Now().Add(adminCacheTTL)}
		adminMu.Unlock()
		return false
	}

	status := member.GetStatus()
	isAdmin := status == "administrator" || status == "creator"

	adminMu.Lock()
	adminCache[key] = adminResult{isAdmin: isAdmin, expires: time.Now().Add(adminCacheTTL)}
	adminMu.Unlock()
	return isAdmin
}
