package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestPacerSpacesPerChatSends(t *testing.T) {
	p := NewPacer()
	p.perChat = 30 * time.Millisecond
	p.global = 5 * time.Millisecond
	start := time.Now()
	// Ten sends to the same chat should each be spaced by ~perChat interval.
	for i := 0; i < 10; i++ {
		p.Wait(100, 0)
	}
	elapsed := time.Since(start)
	// We expect at least (N-1) * perChat, but the send itself is instant here.
	minExpected := 9 * p.perChat
	if elapsed < minExpected {
		t.Fatalf("expected at least %v elapsed for 10 sequential sends to one chat, got %v", minExpected, elapsed)
	}
}

func TestPacerConcurrentSameChat(t *testing.T) {
	p := NewPacer()
	p.perChat = 30 * time.Millisecond
	p.global = 5 * time.Millisecond
	const n = 20
	var wg sync.WaitGroup
	finishes := make(chan time.Duration, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			p.Wait(200, 0)
			finishes <- time.Since(start)
		}()
	}
	wg.Wait()
	close(finishes)

	maxWait := time.Duration(0)
	for d := range finishes {
		if d > maxWait {
			maxWait = d
		}
	}
	// The last of 20 concurrent sends to one chat should wait ~19 intervals.
	minExpected := 19 * p.perChat
	if maxWait < minExpected {
		t.Fatalf("expected last concurrent send to wait ~%v, got %v", minExpected, maxWait)
	}
}

func TestPacerNilIsNoop(t *testing.T) {
	var p *Pacer
	start := time.Now()
	p.Wait(1, 2)
	if elapsed := time.Since(start); elapsed > time.Millisecond {
		t.Fatalf("nil pacer should not block, took %v", elapsed)
	}
	p.Cleanup() // must not panic
}
