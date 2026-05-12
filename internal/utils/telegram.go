package utils

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
)

func IsAdmin(b *gotgbot.Bot, chatID int64, userID int64) bool {
	member, err := b.GetChatMember(chatID, userID, nil)
	if err != nil {
		return false
	}

	switch m := member.(type) {
	case *gotgbot.ChatMemberOwner:
		return true // Owner always has permission
	case *gotgbot.ChatMemberAdministrator:
		return m.CanChangeInfo // Check specific permission
	default:
		return false
	}
}
