package cache

import (
	"sync"
	"time"
)

type item[V any] struct {
	value      V
	expiration time.Time
}

type Cache[K comparable, V any] struct {
	items sync.Map
}

// New creates a new Cache instance
func New[K comparable, V any]() *Cache[K, V] {
	return &Cache[K, V]{}
}

// Set adds an item to the cache with a specific TTL
func (c *Cache[K, V]) Set(key K, value V, ttl time.Duration) {
	c.items.Store(key, item[V]{
		value:      value,
		expiration: time.Now().Add(ttl),
	})
}

// Get retrieves an item from the cache. Returns false if not found or expired.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	val, ok := c.items.Load(key)
	if !ok {
		var zero V
		return zero, false
	}

	itm := val.(item[V])
	if time.Now().After(itm.expiration) {
		c.items.Delete(key)
		var zero V
		return zero, false
	}

	return itm.value, true
}

// Delete removes an item from the cache
func (c *Cache[K, V]) Delete(key K) {
	c.items.Delete(key)
}

// Consume atomically takes an item: it returns the value and true only if the
// key exists and has not expired; the entry is removed either way. Use this
// for single-use tokens (OAuth states) so two concurrent consumers cannot
// both redeem the same token.
func (c *Cache[K, V]) Consume(key K) (V, bool) {
	val, loaded := c.items.LoadAndDelete(key)
	if !loaded {
		var zero V
		return zero, false
	}
	itm := val.(item[V])
	if time.Now().After(itm.expiration) {
		var zero V
		return zero, false
	}
	return itm.value, true
}

// Cleanup removes expired items
func (c *Cache[K, V]) Cleanup() {
	c.items.Range(func(key, value any) bool {
		itm := value.(item[V])
		if time.Now().After(itm.expiration) {
			c.items.Delete(key)
		}
		return true
	})
}

// ClaimSingleUse atomically claims a pre-seeded single-use token. The token is
// seeded with a non-zero issued value (via Set); a successful claim transitions
// the entry to the zero value of V (the "claimed" marker) via compare-and-swap,
// so exactly one presenter can ever win, even under concurrency, without the
// delete-then-reinsert gap that Consume followed by Set would have.
//
// If the key is absent (e.g. the token was issued before a process restart, so
// the in-memory pre-seed was lost, but the token itself is still
// cryptographically valid), the first presenter claims it via LoadOrStore and
// all later presentations are rejected. This keeps the single-use guarantee
// intact across restarts instead of rejecting every valid login.
//
// Expired entries and entries already carrying the zero value (claimed) are
// rejected. V must be comparable and the issued value must be non-zero.
func ClaimSingleUse[K comparable, V comparable](c *Cache[K, V], key K, issued V, claimTTL time.Duration) bool {
	var zero V
	now := time.Now()
	for {
		val, ok := c.items.Load(key)
		if !ok {
			// Absent: claim it. LoadOrStore is atomic, so exactly one racing
			// presenter wins; the zero-value marker is never consumable.
			_, loaded := c.items.LoadOrStore(key, item[V]{value: zero, expiration: now.Add(claimTTL)})
			return !loaded
		}
		itm := val.(item[V])
		if now.After(itm.expiration) || itm.value == zero {
			return false // expired, or already claimed
		}
		if itm.value != issued {
			return false // issued for a different identity
		}
		if c.items.CompareAndSwap(key, val, item[V]{value: zero, expiration: now.Add(claimTTL)}) {
			return true
		}
		// Lost a CAS race with a concurrent claim; retry.
	}
}
