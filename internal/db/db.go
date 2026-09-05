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

	ChatReposCache *cache.Cache[int64, []models.RepoLink]
}

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

	return nil
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

	return nil, errors.New("link not found")
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

	return nil, errors.New("link not found")
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
