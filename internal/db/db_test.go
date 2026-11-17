package db

import (
	"testing"

	"github-webhook/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func fields(v any) map[string]any {
	switch d := v.(type) {
	case bson.D:
		out := make(map[string]any, len(d))
		for _, e := range d {
			out[e.Key] = e.Value
		}
		return out
	case bson.M:
		return map[string]any(d)
	}
	return nil
}

func TestAddRepoLinkUpdate(t *testing.T) {
	link := models.RepoLink{RepoFullName: "owner/repo", WebhookID: 42}
	pipeline := addRepoLinkUpdate(link)
	if len(pipeline) != 1 {
		t.Fatalf("pipeline has %d stages, want 1", len(pipeline))
	}
	raw, err := bson.Marshal(pipeline[0])
	if err != nil {
		t.Fatalf("marshal update stage: %v", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}
	set := fields(fields(doc)["$set"])
	links := fields(set["links"])
	arrays, ok := links["$concatArrays"].(bson.A)
	if !ok || len(arrays) != 2 {
		t.Fatalf("$concatArrays must combine the filtered list and the new link: %#v", links["$concatArrays"])
	}
	stored := fields(arrays[1].(bson.A)[0])
	if stored["repo_full_name"] != link.RepoFullName || stored["webhook_id"] != link.WebhookID {
		t.Fatalf("new link = %#v, want %#v", stored, link)
	}
}
