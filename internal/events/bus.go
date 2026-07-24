package events

import (
	"context"
	"sync"
)

// Bus is a small fan-out broadcaster: one detonation writes, the SSE handler,
// the TUI, the recorder and the oracle engine all read. Subscribers that fall
// behind are dropped rather than allowed to stall a detonation — the chamber
// clock is the thing we must not distort.
type Bus struct {
	mu       sync.RWMutex
	subs     map[int]chan Event
	next     int
	history  []Event
	maxHist  int
	closed   bool
	onDrop   func(sub int)
}

// NewBus returns a bus that replays up to maxHistory events to late joiners.
func NewBus(maxHistory int) *Bus {
	if maxHistory <= 0 {
		maxHistory = 10000
	}
	return &Bus{subs: map[int]chan Event{}, maxHist: maxHistory}
}

// Publish broadcasts to every live subscriber and appends to replay history.
func (b *Bus) Publish(ev Event) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.history = append(b.history, ev)
	if len(b.history) > b.maxHist {
		b.history = b.history[len(b.history)-b.maxHist:]
	}
	targets := make([]chan Event, 0, len(b.subs))
	ids := make([]int, 0, len(b.subs))
	for id, ch := range b.subs {
		targets = append(targets, ch)
		ids = append(ids, id)
	}
	drop := b.onDrop
	b.mu.Unlock()

	for i, ch := range targets {
		select {
		case ch <- ev:
		default:
			if drop != nil {
				drop(ids[i])
			}
		}
	}
}

// Subscribe returns a channel of future events plus a replay of history.
// Cancel the context to unsubscribe.
func (b *Bus) Subscribe(ctx context.Context, buffer int) (<-chan Event, []Event) {
	if buffer <= 0 {
		buffer = 512
	}
	b.mu.Lock()
	id := b.next
	b.next++
	ch := make(chan Event, buffer)
	b.subs[id] = ch
	replay := append([]Event(nil), b.history...)
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		if c, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(c)
		}
		b.mu.Unlock()
	}()
	return ch, replay
}

// History returns a copy of the retained events.
func (b *Bus) History() []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]Event(nil), b.history...)
}

// Close ends all subscriptions.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, ch := range b.subs {
		delete(b.subs, id)
		close(ch)
	}
}
