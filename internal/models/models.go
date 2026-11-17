package models

// User represents a Telegram user linked to a GitHub account
type User struct {
	ID                  int64  `bson:"_id" json:"telegram_id"`
	GitHubUserID        int64  `bson:"github_user_id" json:"github_user_id"`
	GitHubUsername      string `bson:"github_username" json:"github_username"`
	EncryptedOAuthToken string `bson:"encrypted_oauth_token" json:"-"`
}

// RepoLink represents a link to a GitHub repository within a chat
type RepoLink struct {
	RepoFullName string `bson:"repo_full_name" json:"repo_full_name"`
	WebhookID    int64  `bson:"webhook_id,omitempty" json:"webhook_id,omitempty"`
}

// Chat represents a Telegram chat (group, channel, or private)
type Chat struct {
	ID           int64      `bson:"_id" json:"chat_id"`
	ChatType     string     `bson:"chat_type" json:"chat_type"`
	Title        string     `bson:"title" json:"title"`
	Links        []RepoLink `bson:"links" json:"links"`
	MutedThreads []int64    `bson:"muted_threads,omitempty" json:"muted_threads,omitempty"`
}
