package models

import "strconv"

// MessageContext stores the GitHub context associated with a Telegram message ID
type MessageContext struct {
	Owner       string
	Repo        string
	IssueNumber int
	CommentID   int64
	Type        string
}

// MessageContextKey returns the canonical storage and cache key for a message context.
func MessageContextKey(chatID, messageID int64) string {
	return strconv.FormatInt(chatID, 10) + ":" + strconv.FormatInt(messageID, 10)
}

