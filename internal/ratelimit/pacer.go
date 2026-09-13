// Package ratelimit paces outbound Telegram sendMessage calls so that bursts of
// GitHub webhook events are funneled through the API without tripping Telegram's
// rate limits (HTTP 429 "Too Many Requests: retry after N").
//
// Telegram's per-chat limit is roughly one message per second, and bots also
// face a smaller overall per-second allowance across all chats. Rather than
// sending every event the moment it arrives (which is what the original burst
// of check_run/check_suite/workflow_run/workflow_job/deployment_status events
// did, causing the logs to be full of 429s), we schedule each send on a shared
// timeline: per chat (chat_id:thread_id) messages may not start closer together
// than perChat interval, and globally no two sends may start closer together
// than global interval.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

const (
	// defaultPerChatInterval is the minimum spacing between two sends to the SAME
	// chat. 1100ms keeps us safely under Telegram's nominal ~1 msg/sec per chat
	// with room for network round-trips.
	defaultPerChatInterval = 1100 * time.Millisecond
	// defaultGlobalInterval is the minimum spacing between any two sends across
	// all chats. 60ms => at most ~16 msgs/sec bot-wide, well within Telegram's
	// allowance (~30 msgs/sec to different chats).
	defaultGlobalInterval = 60 * time.Millisecond
)

// chatSlot pairs chatID and threadID as a zero-allocation map key.
type chatSlot struct {
	chatID   int64
	threadID int64
}

// Pacer schedules sends to respect Telegram's per-chat and bot-wide limits.
// It is safe for concurrent use.
type Pacer struct {
	mu         sync.Mutex
	globalNext time.Time              // earliest time ANY send may start (bot-wide)
	chatNext   map[chatSlot]time.Time // chat slot -> earliest this chat may send
	perChat    time.Duration
	global     time.Duration
}

// NewPacer creates a Pacer with sensible per-chat and global intervals.
func NewPacer() *Pacer {
	return &Pacer{
		chatNext: make(map[chatSlot]time.Time),
		perChat:  defaultPerChatInterval,
		global:   defaultGlobalInterval,
	}
}

// WaitContext blocks until the current time is past both the per-chat and global
// slots, reserving them for this call, or until ctx is cancelled. Call immediately
// before SendMessage. If p is nil it is a no-op returning nil.
func (p *Pacer) WaitContext(ctx context.Context, chatID, threadID int64) error {
	if p == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	slotKey := chatSlot{chatID: chatID, threadID: threadID}

	p.mu.Lock()
	now := time.Now()
	next := now
	if p.globalNext.After(next) {
		next = p.globalNext
	}
	if slot, ok := p.chatNext[slotKey]; ok && slot.After(next) {
		next = slot
	}
	// Reserve both slots for the next interval.
	p.globalNext = next.Add(p.global)
	p.chatNext[slotKey] = next.Add(p.perChat)
	p.mu.Unlock()

	d := time.Until(next)
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Wait blocks until the current time is past both the per-chat and global slots.
// Preserved for backwards compatibility; prefer WaitContext when a context is available.
func (p *Pacer) Wait(chatID, threadID int64) {
	_ = p.WaitContext(context.Background(), chatID, threadID)
}

// Cleanup drops saved per-chat slots that are no longer in the future, so the
// map does not grow unbounded. Call it periodically (e.g. on the cache sweep).
func (p *Pacer) Cleanup() {
	if p == nil {
		return
	}
	now := time.Now()
	p.mu.Lock()
	for k, slot := range p.chatNext {
		if !slot.After(now) {
			delete(p.chatNext, k)
		}
	}
	p.mu.Unlock()
}
