package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github-webhook/internal/config"
	"github-webhook/internal/models"

	"github-webhook/internal/cache"

	"go.mongodb.org/mongo-driver/v2/bson"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type DB struct {
	Client *mongo.Client
	Users  *mongo.Collection
	Chats  *mongo.Collection
	MsgCtx *mongo.Collection

	ChatReposCache *cache.Cache[int64, []models.RepoLink]
	chatLinksMu    [64]sync.Mutex // bounded stripes serialize cache fills and link writes
}

// ErrLinkNotFound is returned by GetRepoLink/GetRepoLinkByWebhookID when the
// chat exists but the repository is not linked. Callers use errors.Is to
// distinguish "not linked" from a real database failure.
var ErrLinkNotFound = errors.New("link not found")

func Connect(cfg *config.Config) (*DB, error) {
	clientOpts := options.Client().ApplyURI(cfg.MongoDBURI).SetTimeout(15 * time.Second)
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
		Users:          db.Collection("users"),
		Chats:          db.Collection("chats"),
		MsgCtx:         db.Collection("message_contexts"),
		ChatReposCache: cache.New[int64, []models.RepoLink](),
	}

	if err := d.createIndexes(); err != nil {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(disconnectCtx)
		return nil, err
	}

	return d, nil
}

func (d *DB) createIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := d.Chats.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "links.repo_full_name", Value: 1}}},
		{Keys: bson.D{{Key: "links.webhook_id", Value: 1}}},
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
		"_id":          models.MessageContextKey(chatID, messageID),
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
	err := d.MsgCtx.FindOne(ctx, bson.M{"_id": models.MessageContextKey(chatID, messageID)}).Decode(&doc)
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

// AddRepoLink adds a repository link unless the repository is already linked.
// It returns false when an existing link was preserved.
func (d *DB) AddRepoLink(ctx context.Context, chatID int64, link models.RepoLink) (bool, error) {
	defer d.lockChatLinks(chatID)()
	d.invalidateChatLinks(chatID)

	filter := bson.M{"_id": chatID}
	update := mongo.Pipeline{
		{{
			Key: "$set",
			Value: bson.D{{
				Key: "links",
				Value: bson.D{{
					Key: "$cond",
					Value: bson.A{
						bson.D{{Key: "$in", Value: bson.A{
							link.RepoFullName,
							bson.D{{Key: "$ifNull", Value: bson.A{"$links.repo_full_name", bson.A{}}}},
						}}},
						"$links",
						bson.D{{
							Key: "$concatArrays",
							Value: bson.A{
								bson.D{{
									Key:   "$ifNull",
									Value: bson.A{"$links", bson.A{}},
								}},
								bson.A{link},
							},
						}},
					},
				}},
			}},
		}},
	}
	result, err := d.Chats.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return false, err
	}

	d.invalidateChatLinks(chatID)
	return result.ModifiedCount > 0 || result.UpsertedCount > 0, nil
}

// RemoveRepoLink removes a repository link from a chat
func (d *DB) RemoveRepoLink(ctx context.Context, chatID int64, repoFullName string) error {
	defer d.lockChatLinks(chatID)()
	d.invalidateChatLinks(chatID)

	filter := bson.M{"_id": chatID}
	update := bson.M{
		"$pull": bson.M{"links": bson.M{"repo_full_name": repoFullName}},
	}
	_, err := d.Chats.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	d.invalidateChatLinks(chatID)
	return nil
}

// RemoveChatLinks removes every repository link from a chat (used when the chat
// is permanently unreachable, e.g. the bot was blocked). Returns the removed
// links so callers can attempt GitHub-side webhook cleanup.
func (d *DB) RemoveChatLinks(ctx context.Context, chatID int64) ([]models.RepoLink, error) {
	defer d.lockChatLinks(chatID)()
	d.invalidateChatLinks(chatID)

	links, err := d.getChatLinks(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, nil
	}

	// Pull only the exact links observed above. A concurrent AddRepoLink must
	// not be erased merely because it committed between this read and update.
	identities := make(bson.A, 0, len(links))
	for _, link := range links {
		identity := bson.M{"repo_full_name": link.RepoFullName}
		if link.WebhookID != 0 {
			identity["webhook_id"] = link.WebhookID
		}
		identities = append(identities, bson.M{"$or": bson.A{identity}})
	}
	_, err = d.Chats.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{"$pull": bson.M{"links": bson.M{"$or": identities}}})
	if err != nil {
		return nil, err
	}

	d.invalidateChatLinks(chatID)
	return links, nil
}

// GetChatLinks returns all repository links for a chat.
func (d *DB) GetChatLinks(ctx context.Context, chatID int64) ([]models.RepoLink, error) {
	defer d.lockChatLinks(chatID)()
	return d.getChatLinks(ctx, chatID)
}

func (d *DB) getChatLinks(ctx context.Context, chatID int64) ([]models.RepoLink, error) {
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

// lockChatLinks serializes cache fills and link mutations for one chat.
func (d *DB) lockChatLinks(chatID int64) func() {
	mu := &d.chatLinksMu[uint64(chatID)%uint64(len(d.chatLinksMu))]
	mu.Lock()
	return mu.Unlock
}

// invalidateChatLinks must be called while lockChatLinks is held by a writer.
func (d *DB) invalidateChatLinks(chatID int64) {
	d.ChatReposCache.Delete(chatID)
}

// findLink scans the chat's cached links for the first one matching match.
// Returns a copy of the link (never a pointer into the cached slice, which
// callers could mutate) or ErrLinkNotFound.
func (d *DB) findLink(ctx context.Context, chatID int64, match func(models.RepoLink) bool) (*models.RepoLink, error) {
	links, err := d.GetChatLinks(ctx, chatID)
	if err != nil {
		return nil, err
	}

	for _, link := range links {
		if match(link) {
			found := link
			return &found, nil
		}
	}

	return nil, ErrLinkNotFound
}

// GetRepoLink returns a specific repository link for a chat
func (d *DB) GetRepoLink(ctx context.Context, chatID int64, repoFullName string) (*models.RepoLink, error) {
	return d.findLink(ctx, chatID, func(l models.RepoLink) bool {
		return l.RepoFullName == repoFullName
	})
}

// GetRepoLinkByWebhookID returns a specific repository link by webhook ID
func (d *DB) GetRepoLinkByWebhookID(ctx context.Context, chatID int64, webhookID int64) (*models.RepoLink, error) {
	return d.findLink(ctx, chatID, func(l models.RepoLink) bool {
		return l.WebhookID == webhookID
	})
}

// UpdateRepoLinkName updates the repository name for a given webhook ID in a chat
func (d *DB) UpdateRepoLinkName(ctx context.Context, chatID int64, webhookID int64, newRepoFullName string) error {
	defer d.lockChatLinks(chatID)()
	d.invalidateChatLinks(chatID)

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

	return nil
}
