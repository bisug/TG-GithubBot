package utils

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github-webhook/internal/cache"

	"github.com/AshokShau/gotdbot"
)

// IsIgnorableTelegramError reports whether an error is a benign Telegram UI race condition,
// such as an edit request for identical content or a superseded edit request.
func IsIgnorableTelegramError(err error) bool {
	if err == nil {
		return false
	}
	var msg string
	if tgErr, ok := errors.AsType[*gotdbot.Error](err); ok && tgErr != nil {
		msg = strings.ToLower(tgErr.Message)
	} else if tgErr, ok := errors.AsType[gotdbot.Error](err); ok {
		msg = strings.ToLower(tgErr.Message)
	} else {
		msg = strings.ToLower(err.Error())
	}

	return strings.Contains(msg, "message_not_modified") ||
		strings.Contains(msg, "message is not modified") ||
		strings.Contains(msg, "canceled by new edit message request") ||
		strings.Contains(msg, "query_id_invalid")
}

func IsAdmin(b *gotdbot.Client, chatID int64, userID int64, adminCache *cache.Cache[int64, []int64]) bool {
	if admins, ok := adminCache.Get(chatID); ok {
		return slices.Contains(admins, userID)
	}

	admins, err := b.GetChatAdministrators(chatID)
	if err != nil {
		b.Logger.Warnf("Read Telegram administrators: %s", ErrorDetails(err))
		return false
	}

	var adminIDs []int64
	isAdmin := false
	for _, admin := range admins.Administrators {
		id := admin.UserId
		adminIDs = append(adminIDs, id)
		if id == userID {
			isAdmin = true
		}
	}

	adminCache.Set(chatID, adminIDs, 1*time.Hour)
	return isAdmin
}
