package commands

import (
	"context"
	"fmt"
	"strings"

	"github-webhook/internal/cache"
	"github-webhook/internal/db"
	"github-webhook/internal/github"
	"github-webhook/internal/models"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	gh "github.com/google/go-github/v85/github"
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
		if err.Error() == "unauthorized" {
			_, _ = msg.Reply(b, "Please /connect your GitHub account in a private chat before replying to GitHub items.", nil)
		} else {
			_, _ = msg.Reply(b, "Auth error. Reconnect via /connect", nil)
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
		fmt.Printf("Failed to post comment to %s/%s#%d: %v\n", mContext.Owner, mContext.Repo, mContext.IssueNumber, err)
		return nil
	}

	return nil
}
