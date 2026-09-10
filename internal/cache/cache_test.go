package cache

import (
	"testing"
	"time"
)

// TestConsumeSingleUse verifies that Consume atomically claims an item exactly
// once, so single-use tokens (OAuth states) cannot be redeemed twice.
func TestConsumeSingleUse(t *testing.T) {
	c := New[string, int]()
	c.Set("token", 42, 10 * time.Minute)

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
	c.Set("token", 7, 10 * time.Minute)

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