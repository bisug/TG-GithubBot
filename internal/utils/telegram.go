package utils

import (
	"github-webhook/internal/cache"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

func IsAdmin(b *gotgbot.Bot, chatID int64, userID int64, adminCache *cache.Cache[int64, []int64]) bool {
	if adminCache != nil {
		if admins, ok := adminCache.Get(chatID); ok {
			for _, id := range admins {
				if id == userID {
					return true
				}
			}
			return false
		}
	}

	// Fetch admins from Telegram
	admins, err := b.GetChatAdministrators(chatID, nil)
	if err != nil {
		// Fallback to single member check if GetChatAdministrators fails
		member, err := b.GetChatMember(chatID, userID, nil)
		if err != nil {
			return false
		}
		status := member.GetStatus()
		return status == "administrator" || status == "creator"
	}

	var adminIDs []int64
	isAdmin := false
	for _, admin := range admins {
		var id int64
		switch m := admin.(type) {
		case *gotgbot.ChatMemberOwner:
			id = m.User.Id
		case *gotgbot.ChatMemberAdministrator:
			id = m.User.Id
		default:
			continue
		}
		adminIDs = append(adminIDs, id)
		if id == userID {
			isAdmin = true
		}
	}

	if adminCache != nil {
		adminCache.Set(chatID, adminIDs, 30*time.Minute)
	}

	return isAdmin
}
