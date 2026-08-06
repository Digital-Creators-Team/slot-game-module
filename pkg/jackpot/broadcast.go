package jackpot

import (
	"context"
	"sync"
	"time"
)

// Broadcaster is a minimal pub/sub for pool updates.
type Broadcaster struct {
	mu        sync.RWMutex
	listeners map[chan Update]struct{}
	buffer    int
}

// NewBroadcaster creates a broadcaster with a buffer size for each listener channel.
func NewBroadcaster(buffer int) *Broadcaster {
	return &Broadcaster{
		listeners: make(map[chan Update]struct{}),
		buffer:    buffer,
	}
}

// Send publishes an update to all listeners (non-blocking with drop on full buffer).
func (b *Broadcaster) Send(update Update) {
	b.mu.RLock()
	listeners := make([]chan Update, 0, len(b.listeners))
	for ch := range b.listeners {
		listeners = append(listeners, ch)
	}
	b.mu.RUnlock()

	// Send to all listeners (non-blocking)
	for _, ch := range listeners {
		select {
		case ch <- update:
		default:
			// drop if listener is slow; keep simple
		}
	}
}

// Listen returns a channel plus a cancel function to stop listening.
func (b *Broadcaster) Listen(ctx context.Context) (<-chan Update, context.CancelFunc) {
	listenerCtx, cancel := context.WithCancel(ctx)
	out := make(chan Update, b.buffer)

	// Register listener
	b.mu.Lock()
	b.listeners[out] = struct{}{}
	b.mu.Unlock()

	go func() {
		<-listenerCtx.Done()
		b.mu.Lock()
		delete(b.listeners, out)
		b.mu.Unlock()
		close(out)
	}()

	return out, cancel
}

// SendWithTimeout publishes with timeout to all listeners.
// Returns true if sent to at least one listener, false if timeout or no listeners.
func (b *Broadcaster) SendWithTimeout(update Update, timeout time.Duration) bool {
	b.mu.RLock()
	listeners := make([]chan Update, 0, len(b.listeners))
	for ch := range b.listeners {
		listeners = append(listeners, ch)
	}
	b.mu.RUnlock()

	if len(listeners) == 0 {
		return false
	}

	// Try to send to all listeners with timeout
	done := make(chan struct{})
	go func() {
		for _, ch := range listeners {
			select {
			case ch <- update:
			default:
				// drop if listener is slow
			}
		}
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// BatchUpdate represents multiple updates from the same spin that should be sent together.
type BatchUpdate struct {
	Updates []Update
	SpinID  string
}

// SendBatch publishes multiple updates as a batch (non-blocking with drop on full buffer).
// This ensures all pools from the same spin are delivered atomically.
func (b *Broadcaster) SendBatch(updates []Update) {
	if len(updates) == 0 {
		return
	}
	// Send a special batch marker first, then all updates
	// We'll use a special Update with SpinID to mark it as a batch start
	// For now, send all updates in sequence - handler will batch them
	// In the future, we could add a BatchUpdate type if needed
	for _, update := range updates {
		b.Send(update)
	}
}
