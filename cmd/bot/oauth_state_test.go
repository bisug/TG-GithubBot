package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github-webhook/internal/cache"
	"github-webhook/internal/utils"
)

func TestResolveOAuthState(t *testing.T) {
	const encKey = "0123456789abcdef0123456789abcdef"
	stateCache := cache.New[string, int64]()

	// 1. Missing state
	if _, err := resolveOAuthState("", stateCache, encKey); err == nil {
		t.Fatal("expected error for empty state")
	}

	// 2. Cached state
	stateCache.Set("valid_cached_state", 12345, 10*time.Minute)
	id, err := resolveOAuthState("valid_cached_state", stateCache, encKey)
	if err != nil || id != 12345 {
		t.Fatalf("resolveOAuthState(cached) = (%d, %v), want (12345, nil)", id, err)
	}

	// 3. Already claimed state in cache (0)
	stateCache.Set("claimed_state", 0, 10*time.Minute)
	if _, err := resolveOAuthState("claimed_state", stateCache, encKey); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("expected 'already used' error for claimed state, got: %v", err)
	}

	// 4. Valid encrypted state
	nowPayload := fmt.Sprintf("98765:%d:randomnonce", time.Now().Unix())
	encValid, err := utils.Encrypt(nowPayload, encKey)
	if err != nil {
		t.Fatalf("failed to encrypt test payload: %v", err)
	}
	id, err = resolveOAuthState(encValid, stateCache, encKey)
	if err != nil || id != 98765 {
		t.Fatalf("resolveOAuthState(encrypted) = (%d, %v), want (98765, nil)", id, err)
	}

	// 5. Expired encrypted state (15 minutes old)
	oldPayload := fmt.Sprintf("98765:%d:randomnonce", time.Now().Add(-15*time.Minute).Unix())
	encOld, err := utils.Encrypt(oldPayload, encKey)
	if err != nil {
		t.Fatalf("failed to encrypt old payload: %v", err)
	}
	if _, err := resolveOAuthState(encOld, stateCache, encKey); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got: %v", err)
	}

	// 6. Malformed encrypted state
	malformedPayload := "just_one_part"
	encMalformed, _ := utils.Encrypt(malformedPayload, encKey)
	if _, err := resolveOAuthState(encMalformed, stateCache, encKey); err == nil || !strings.Contains(err.Error(), "invalid state format") {
		t.Fatalf("expected invalid format error, got: %v", err)
	}

	// 7. Invalid telegram ID in payload
	invalidIDPayload := fmt.Sprintf("notanumber:%d:nonce", time.Now().Unix())
	encInvalidID, _ := utils.Encrypt(invalidIDPayload, encKey)
	if _, err := resolveOAuthState(encInvalidID, stateCache, encKey); err == nil || !strings.Contains(err.Error(), "invalid telegram id") {
		t.Fatalf("expected invalid telegram id error, got: %v", err)
	}
}
