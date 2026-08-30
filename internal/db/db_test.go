package db

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github-webhook/internal/models"
)

// TestBuildUserUpsertShape locks the UpsertUser BSON shape without requiring a live
// MongoDB: _id must live under $setOnInsert (never $set, which MongoDB rejects for
// existing documents) and the document must be valid BSON.
func TestBuildUserUpsertShape(t *testing.T) {
	update := buildUserUpsert(&models.User{
		ID:                  42,
		GitHubUserID:        7,
		GitHubUsername:      "alice",
		EncryptedOAuthToken: "enc",
		Scopes:              []string{"repo"},
	})

	if _, ok := update["$set"]; !ok {
		t.Fatal("update missing $set")
	}
	if _, ok := update["$setOnInsert"]; !ok {
		t.Fatal("update missing $setOnInsert")
	}

	set := update["$set"].(bson.M)
	if _, ok := set["_id"]; ok {
		t.Fatal("_id must not appear under $set (immutable field)")
	}

	setOnInsert := update["$setOnInsert"].(bson.M)
	id, ok := setOnInsert["_id"]
	if !ok || id != int64(42) {
		t.Fatalf("expected _id=42 under $setOnInsert, got %v (present=%v)", id, ok)
	}

	data, err := bson.Marshal(update)
	if err != nil {
		t.Fatalf("update is not valid BSON: %v", err)
	}
	var round bson.M
	if err := bson.Unmarshal(data, &round); err != nil {
		t.Fatalf("bson round-trip failed: %v", err)
	}
}
