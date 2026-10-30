package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCacheConcurrentAccess(t *testing.T) {
	c := New[string, int]()
	const numGoroutines = 50
	const numOps = 200

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3)

	// Writer goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				key := fmt.Sprintf("key-%d", j%20)
				c.Set(key, id*1000+j, 100*time.Millisecond)
			}
		}(i)
	}

	// Reader goroutines
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				key := fmt.Sprintf("key-%d", j%20)
				_, _ = c.Get(key)
			}
		}()
	}

	// Deleter / Cleaner goroutines
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				if j%2 == 0 {
					key := fmt.Sprintf("key-%d", j%20)
					c.Delete(key)
				} else {
					c.Cleanup()
				}
			}
		}()
	}

	wg.Wait()
}
