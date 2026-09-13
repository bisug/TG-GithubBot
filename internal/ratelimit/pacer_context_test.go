package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPacerWaitContextSuccess(t *testing.T) {
	p := NewPacer()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := p.WaitContext(ctx, 100, 0)
	if err != nil {
		t.Fatalf("WaitContext failed: %v", err)
	}
}

func TestPacerWaitContextAlreadyCancelled(t *testing.T) {
	p := NewPacer()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := p.WaitContext(ctx, 100, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitContext on cancelled context = %v, want context.Canceled", err)
	}
}

func TestPacerWaitContextCancellationDuringWait(t *testing.T) {
	p := &Pacer{
		chatNext: make(map[chatSlot]time.Time),
		perChat:  500 * time.Millisecond,
		global:   100 * time.Millisecond,
	}

	// First call consumes the slot immediately
	if err := p.WaitContext(context.Background(), 200, 0); err != nil {
		t.Fatalf("first WaitContext failed: %v", err)
	}

	// Second call would wait 500ms, but we cancel after 50ms
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := p.WaitContext(ctx, 200, 0)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitContext = %v, want context.DeadlineExceeded", err)
	}
	if elapsed >= 400*time.Millisecond {
		t.Fatalf("WaitContext did not abort early on deadline exceeded; took %v", elapsed)
	}
}

func TestPacerNilSafety(t *testing.T) {
	var p *Pacer
	if err := p.WaitContext(context.Background(), 1, 1); err != nil {
		t.Fatalf("nil Pacer WaitContext returned error: %v", err)
	}
	p.Wait(1, 1)
	p.Cleanup()
}
