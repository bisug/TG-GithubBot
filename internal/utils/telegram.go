package utils

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
)

func IsAdmin(b *gotgbot.Bot, chatID int64, userID int64) bool {
	member, err := b.GetChatMember(chatID, userID, nil)
	if err != nil {
		return false
	}

	status := member.GetStatus()
	return status == "administrator" || status == "creator"
}
