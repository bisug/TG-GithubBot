package cache

import (
	"testing"
	"time"
)

// TestConsumeSingleUse verifies that Consume atomically claims an item exactly
// once, so single-use tokens (OAuth states) cannot be redeemed twice.
func TestConsumeSingleUse(t *testing.T) {
	c := New[string, int]()
	c.Set("token", 42, 10*time.Minute)

	val, ok := c.Consume("token")
	if !ok || val != 42 {
		t.Fatalf("first Consume should return value, ok; got %v, %v", val, ok)
	}

	_, ok = c.Consume("token")
	if ok {
		t.Fatal("second Consume of the same key must fail (already consumed)")
	}

	_, ok = c.Get("token")
	if ok {
		t.Fatal("key should have been removed by Consume")
	}
}

// TestConsumeExpired verifies Consume rejects entries that have already expired.
func TestConsumeExpired(t *testing.T) {
	c := New[string, int]()
	c.Set("token", 1, time.Nanosecond)

	time.Sleep(time.Millisecond)
	_, ok := c.Consume("token")
	if ok {
		t.Fatal("Consume must reject an expired entry")
	}
}

// TestConsumeConcurrent verifies that when many goroutines race to Consume the
// same key, exactly one wins.
func TestConsumeConcurrent(t *testing.T) {
	c := New[string, int]()
	c.Set("token", 7, 10*time.Minute)

	results := make(chan bool, 32)
	for i := 0; i < 16; i++ {
		go func() {
			_, ok := c.Consume("token")
			results <- ok
		}()
	}

	wins := 0
	for i := 0; i < 16; i++ {
		var ok bool
		select {
		case ok = <-results:
		}
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly one winner, got %d", wins)
	}
}

// TestAddIfAbsent verifies that AddIfAbsent claims a key exactly once under
// concurrency (best-effort dedup semantics).
func TestAddIfAbsent(t *testing.T) {
	c := New[string, struct{}]()

	if !c.AddIfAbsent("d1", struct{}{}, 10*time.Minute) {
		t.Fatal("first AddIfAbsent must succeed")
	}
	if c.AddIfAbsent("d1", struct{}{}, 10*time.Minute) {
		t.Fatal("second AddIfAbsent for the same key must fail")
	}
	if _, ok := c.Get("d1"); !ok {
		t.Fatal("key should be retrievable after AddIfAbsent")
	}

	results := make(chan bool, 32)
	for i := 0; i < 16; i++ {
		go func() { results <- c.AddIfAbsent("d2", struct{}{}, 10*time.Minute) }()
	}
	wins := 0
	for i := 0; i < 16; i++ {
		if <-results {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly one winner, got %d", wins)
	}
}

// TestClaimSingleUse verifies that ClaimSingleUse allows exactly one claim of a
// pre-seeded token, and that replays are rejected afterwards.
func TestClaimSingleUse(t *testing.T) {
	c := New[string, int64]()
	c.Set("token", 42, 10*time.Minute)

	if !ClaimSingleUse(c, "token", 42, 10*time.Minute) {
		t.Fatal("first claim of a pre-seeded token must succeed")
	}
	if ClaimSingleUse(c, "token", 42, 10*time.Minute) {
		t.Fatal("second claim of the same token must fail (already claimed)")
	}
	if ClaimSingleUse(c, "token", 43, 10*time.Minute) {
		t.Fatal("claim with a mismatched issued value must fail")
	}
}

// TestClaimSingleUseAbsent verifies the restart-recovery path: a token that was
// never seeded in this process (e.g. issued before a restart) is claimed on
// first presentation and rejected on every later one.
func TestClaimSingleUseAbsent(t *testing.T) {
	c := New[string, int64]()

	if !ClaimSingleUse(c, "token", 42, 10*time.Minute) {
		t.Fatal("first claim of an unseeded token must succeed")
	}
	if ClaimSingleUse(c, "token", 42, 10*time.Minute) {
		t.Fatal("second claim of an unseeded token must fail (already claimed)")
	}
}

// TestClaimSingleUseExpired verifies expired pre-seeded entries are rejected
// and cannot be claimed.
func TestClaimSingleUseExpired(t *testing.T) {
	c := New[string, int64]()
	c.Set("token", 42, time.Nanosecond)

	time.Sleep(time.Millisecond)
	if ClaimSingleUse(c, "token", 42, 10*time.Minute) {
		t.Fatal("claim of an expired token must fail")
	}
}

// TestClaimSingleUseConcurrent verifies that when many goroutines race to claim
// the same token, exactly one wins — both for pre-seeded and unseeded tokens.
func TestClaimSingleUseConcurrent(t *testing.T) {
	for _, seeded := range []bool{true, false} {
		c := New[string, int64]()
		if seeded {
			c.Set("token", 7, 10*time.Minute)
		}

		results := make(chan bool, 32)
		for i := 0; i < 16; i++ {
			go func() {
				results <- ClaimSingleUse(c, "token", 7, 10*time.Minute)
			}()
		}

		wins := 0
		for i := 0; i < 16; i++ {
			if <-results {
				wins++
			}
		}
		if wins != 1 {
			t.Fatalf("seeded=%v: expected exactly one winner, got %d", seeded, wins)
		}
	}
}
