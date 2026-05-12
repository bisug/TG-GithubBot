package github

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v85/github"
)

func TriggerRepositoryHookTest(ctx context.Context, client *gh.Client, owner, repo string, hookID int64) error {
	if hookID == 0 {
		return fmt.Errorf("missing webhook id")
	}

	_, err := client.Repositories.TestHook(ctx, owner, repo, hookID)
	if err != nil {
		return err
	}

	return nil
}

func TriggerRepositoryHookPing(ctx context.Context, client *gh.Client, owner, repo string, hookID int64) error {
	if hookID == 0 {
		return fmt.Errorf("missing webhook id")
	}

	_, err := client.Repositories.PingHook(ctx, owner, repo, hookID)
	if err != nil {
		return err
	}

	return nil
}
