package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github-webhook/internal/config"
	"github-webhook/internal/models"

	"github-webhook/internal/cache"

	"go.mongodb.org/mongo-driver/v2/bson"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type DB struct {
	Client   *mongo.Client
	Database *mongo.Database
	Users    *mongo.Collection
	Chats    *mongo.Collection
	MsgCtx   *mongo.Collection

	ChatReposCache *cache.Cache[int64, []models.RepoLink]
}

// ErrLinkNotFound is returned by GetRepoLink/GetRepoLinkByWebhookID when the
// chat exists but the repository is not linked. Callers use errors.Is to
// distinguish "not linked" from a real database failure.
var ErrLinkNotFound = errors.New("link not found")

func Connect(cfg *config.Config) (*DB, error) {
	clientOpts := options.Client().ApplyURI(cfg.MongoDBURI)
	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, err
	}

	// mongo.Connect is lazy; force a real connection with retries so transient
	// Atlas unavailability (e.g. a paused M0 cluster waking up) doesn't crash
	// the process on the first deploy or after a cold start.
	const attempts = 4
	var pingErr error
	for i := 1; i <= attempts; i++ {
		pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		pingErr = client.Ping(pingCtx, nil)
		cancel()
		if pingErr == nil {
			break
		}
		slog.Warn("MongoDB ping failed", "attempt", i, "of", attempts, "error", pingErr)
		if i < attempts {
			time.Sleep(time.Duration(i*3) * time.Second) // 3s, 6s, 9s backoff
		}
	}
	if pingErr != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("connect to DB after %d attempts: %w", attempts, pingErr)
	}

	db := client.Database(cfg.DatabaseName)

	d := &DB{
		Client:         client,
		Database:       db,
		Users:          db.Collection("users"),
		Chats:          db.Collection("chats"),
		MsgCtx:         db.Collection("message_contexts"),
		ChatReposCache: cache.New[int64, []models.RepoLink](),
	}

	if err := d.createIndexes(); err != nil {
		return nil, err
	}

	return d, nil
}

func (d *DB) createIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := d.Chats.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "links.repo_full_name", Value: 1}},
	})

	if err != nil {
		return err
	}

	_, err = d.Chats.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "links.webhook_id", Value: 1}},
	})
	if err != nil {
		return err
	}

	// Message contexts expire server-side after 48h so reply-to-comment and
	// /close /reopen /approve keep working across restarts without manual cleanup.
	_, err = d.MsgCtx.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})

	return err
}

func (d *DB) GetUserByTelegramID(ctx context.Context, telegramID int64) (*models.User, error) {
	var user models.User
	err := d.Users.FindOne(ctx, bson.M{"_id": telegramID}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (d *DB) UpsertUser(ctx context.Context, user *models.User) error {
	opts := options.UpdateOne().SetUpsert(true)
	filter := bson.M{"_id": user.ID}
	_, err := d.Users.UpdateOne(ctx, filter, buildUserUpsert(user), opts)
	return err
}

// buildUserUpsert returns the update document for UpsertUser. Mutable fields go under
// $set; the immutable _id goes under $setOnInsert so re-upserts of existing users never
// attempt to modify _id (which MongoDB rejects).
func buildUserUpsert(user *models.User) bson.M {
	return bson.M{
		"$set": bson.M{
			"github_user_id":        user.GitHubUserID,
			"github_username":       user.GitHubUsername,
			"encrypted_oauth_token": user.EncryptedOAuthToken,
			"scopes":                user.Scopes,
		},
		"$setOnInsert": bson.M{
			"_id": user.ID,
		},
	}
}

func (d *DB) ClearUserToken(ctx context.Context, userID int64) error {
	filter := bson.M{"_id": userID}
	update := bson.M{"$set": bson.M{"encrypted_oauth_token": ""}}
	_, err := d.Users.UpdateOne(ctx, filter, update)
	return err
}

// msgCtxTTL is how long a stored message context stays valid. Matches the
// in-memory cache TTL so both layers age out together.
const msgCtxTTL = 48 * time.Hour

// StoreMessageContext persists the GitHub context for a sent notification so
// reply actions (/close, /reopen, /approve, reply-to-comment) survive restarts.
// Keyed by chat_id + message_id.
func (d *DB) StoreMessageContext(ctx context.Context, chatID, messageID int64, mc models.MessageContext) error {
	doc := bson.M{
		"_id":          fmt.Sprintf("%d:%d", chatID, messageID),
		"owner":        mc.Owner,
		"repo":         mc.Repo,
		"issue_number": mc.IssueNumber,
		"comment_id":   mc.CommentID,
		"type":         mc.Type,
		"expires_at":   time.Now().Add(msgCtxTTL),
	}
	_, err := d.MsgCtx.ReplaceOne(ctx, bson.M{"_id": doc["_id"]}, doc, options.Replace().SetUpsert(true))
	return err
}

// GetMessageContext retrieves a stored message context by chat and message ID.
// Returns mongo.ErrNoDocuments when absent or expired.
func (d *DB) GetMessageContext(ctx context.Context, chatID, messageID int64) (models.MessageContext, error) {
	var doc struct {
		Owner       string `bson:"owner"`
		Repo        string `bson:"repo"`
		IssueNumber int    `bson:"issue_number"`
		CommentID   int64  `bson:"comment_id"`
		Type        string `bson:"type"`
	}
	err := d.MsgCtx.FindOne(ctx, bson.M{"_id": fmt.Sprintf("%d:%d", chatID, messageID)}).Decode(&doc)
	if err != nil {
		return models.MessageContext{}, err
	}
	return models.MessageContext{
		Owner:       doc.Owner,
		Repo:        doc.Repo,
		IssueNumber: doc.IssueNumber,
		CommentID:   doc.CommentID,
		Type:        doc.Type,
	}, nil
}

func (d *DB) GetChat(ctx context.Context, chatID int64) (*models.Chat, error) {
	var chat models.Chat
	err := d.Chats.FindOne(ctx, bson.M{"_id": chatID}).Decode(&chat)
	if err != nil {
		return nil, err
	}
	return &chat, nil
}

func (d *DB) UpsertChat(ctx context.Context, chat *models.Chat) error {
	opts := options.UpdateOne().SetUpsert(true)
	filter := bson.M{"_id": chat.ID}

	update := bson.M{
		"$set": bson.M{
			"title":     chat.Title,
			"chat_type": chat.ChatType,
		},
	}
	_, err := d.Chats.UpdateOne(ctx, filter, update, opts)
	return err
}

// AddRepoLink adds a repository link to a chat
func (d *DB) AddRepoLink(ctx context.Context, chatID int64, link models.RepoLink) error {
	filter := bson.M{"_id": chatID}
	update := mongo.Pipeline{
		{{
			Key: "$set",
			Value: bson.D{{
				Key: "links",
				Value: bson.D{{
					Key: "$concatArrays",
					Value: bson.A{
						bson.D{{
							Key: "$filter",
							Value: bson.D{
								{Key: "input", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$links", bson.A{}}}}},
								{Key: "as", Value: "link"},
								{Key: "cond", Value: bson.D{{Key: "$ne", Value: bson.A{"$$link.repo_full_name", link.RepoFullName}}}},
							},
						}},
						bson.A{link},
					},
				}},
			}},
		}},
	}
	_, err := d.Chats.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))

	d.ChatReposCache.Delete(chatID)
	return err
}

// RemoveRepoLink removes a repository link from a chat
func (d *DB) RemoveRepoLink(ctx context.Context, chatID int64, repoFullName string) error {
	filter := bson.M{"_id": chatID}
	update := bson.M{
		"$pull": bson.M{"links": bson.M{"repo_full_name": repoFullName}},
	}
	_, err := d.Chats.UpdateOne(ctx, filter, update)

	d.ChatReposCache.Delete(chatID)
	return err
}

// RemoveChatLinks removes every repository link from a chat (used when the chat
// is permanently unreachable, e.g. the bot was blocked). Returns the removed
// links so callers can attempt GitHub-side webhook cleanup.
func (d *DB) RemoveChatLinks(ctx context.Context, chatID int64) ([]models.RepoLink, error) {
	links, err := d.GetChatLinks(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, nil
	}

	_, err = d.Chats.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{"$set": bson.M{"links": bson.A{}}})
	if err != nil {
		return nil, err
	}

	d.ChatReposCache.Delete(chatID)
	return links, nil
}

// GetChatLinks returns all repository links for a chat
func (d *DB) GetChatLinks(ctx context.Context, chatID int64) ([]models.RepoLink, error) {
	if cached, ok := d.ChatReposCache.Get(chatID); ok {
		return cached, nil
	}

	chat, err := d.GetChat(ctx, chatID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return []models.RepoLink{}, nil
		}
		return nil, err
	}

	d.ChatReposCache.Set(chatID, chat.Links, 30*time.Minute)
	return chat.Links, nil
}

// GetRepoLink returns a specific repository link for a chat
func (d *DB) GetRepoLink(ctx context.Context, chatID int64, repoFullName string) (*models.RepoLink, error) {
	links, err := d.GetChatLinks(ctx, chatID)
	if err != nil {
		return nil, err
	}

	for _, link := range links {
		if link.RepoFullName == repoFullName {
			return &link, nil
		}
	}

	return nil, ErrLinkNotFound
}

// GetRepoLinkByWebhookID returns a specific repository link by webhook ID
func (d *DB) GetRepoLinkByWebhookID(ctx context.Context, chatID int64, webhookID int64) (*models.RepoLink, error) {
	links, err := d.GetChatLinks(ctx, chatID)
	if err != nil {
		return nil, err
	}

	for _, link := range links {
		if link.WebhookID == webhookID {
			return &link, nil
		}
	}

	return nil, ErrLinkNotFound
}

// UpdateRepoLinkName updates the repository name for a given webhook ID in a chat
func (d *DB) UpdateRepoLinkName(ctx context.Context, chatID int64, webhookID int64, newRepoFullName string) error {
	filter := bson.M{
		"_id":              chatID,
		"links.webhook_id": webhookID,
	}
	update := bson.M{
		"$set": bson.M{"links.$.repo_full_name": newRepoFullName},
	}

	result, err := d.Chats.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return errors.New("no matching link found to update")
	}

	if cachedLinks, ok := d.ChatReposCache.Get(chatID); ok {
		newLinks := make([]models.RepoLink, len(cachedLinks))
		copy(newLinks, cachedLinks)

		updated := false
		for i, link := range newLinks {
			if link.WebhookID == webhookID {
				newLinks[i].RepoFullName = newRepoFullName
				updated = true
				break
			}
		}

		if updated {
			d.ChatReposCache.Set(chatID, newLinks, 30*time.Minute)
		} else {
			d.ChatReposCache.Delete(chatID)
		}
	}

	return nil
}
