package commands

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github-webhook/internal/cache"
	"github-webhook/internal/db"
	"github-webhook/internal/github"
	"github-webhook/internal/models"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	gh "github.com/google/go-github/v90/github"
)

type ReplyHandler struct {
	DB            *db.DB
	ClientFactory *github.ClientFactory
	EncryptionKey string
	ContextCache  *cache.Cache[string, models.MessageContext]
}

func NewReplyHandler(database *db.DB, factory *github.ClientFactory, key string, ctxCache *cache.Cache[string, models.MessageContext]) *ReplyHandler {
	return &ReplyHandler{
		DB:            database,
		ClientFactory: factory,
		EncryptionKey: key,
		ContextCache:  ctxCache,
	}
}

func (h *ReplyHandler) HandleReply(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if msg.ReplyToMessage == nil {
		return nil
	}

	key := fmt.Sprintf("%d:%d", ctx.EffectiveChat.Id, msg.ReplyToMessage.MessageId)
	mContext, found := h.ContextCache.Get(key)
	if !found {
		return nil
	}

	commentBody := strings.TrimSpace(msg.Text)
	if commentBody == "" {
		return nil
	}

	client, err := github.GetClientForUser(context.Background(), h.DB, h.ClientFactory, ctx.EffectiveUser.Id, h.EncryptionKey)
	if err != nil {
		if errors.Is(err, github.ErrUnauthorized) {
			_, _ = msg.Reply(b, "Please /connect your GitHub account in a private chat before replying to GitHub items.", nil)
		} else {
			_, _ = msg.Reply(b, "⚠️ <b>Authentication error.</b>\nPlease reconnect your GitHub account using /connect in a private chat.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		}
		return nil
	}
	if mContext.Type == "pr_review_comment" && mContext.CommentID != 0 {
		comment := &gh.PullRequestComment{
			Body:      &commentBody,
			InReplyTo: &mContext.CommentID,
		}
		_, _, err = client.PullRequests.CreateComment(context.Background(), mContext.Owner, mContext.Repo, mContext.IssueNumber, comment)
	} else {
		comment := &gh.IssueComment{Body: &commentBody}
		_, _, err = client.Issues.CreateComment(context.Background(), mContext.Owner, mContext.Repo, mContext.IssueNumber, comment)
	}

	if err != nil {
		slog.Error("Failed to post comment", "owner", mContext.Owner, "repo", mContext.Repo, "issue", mContext.IssueNumber, "error", err)
		_, _ = msg.Reply(b, "❌ <b>Failed to post your comment on GitHub.</b>\nPlease try again, or add it directly on GitHub.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return nil
	}

	// Confirm with a 👍 reaction on the user's message; fall back to a text
	// confirmation if reactions are unavailable (e.g. some channel configs).
	if _, err := b.SetMessageReaction(ctx.EffectiveChat.Id, msg.MessageId, &gotgbot.SetMessageReactionOpts{
		Reaction: []gotgbot.ReactionType{gotgbot.ReactionTypeEmoji{Emoji: "👍"}},
	}); err != nil {
		slog.Debug("Could not set comment confirmation reaction", "chat", ctx.EffectiveChat.Id, "error", err)
		_, _ = msg.Reply(b, "💬 Comment posted on GitHub.", nil)
	}
	return nil
}
