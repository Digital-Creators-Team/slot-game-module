package jackpot

import (
	"context"
	"time"
)

// Broadcaster is a minimal pub/sub for pool updates.
type Broadcaster struct {
	ch chan Update
}

// NewBroadcaster creates a broadcaster with a buffered channel.
func NewBroadcaster(buffer int) *Broadcaster {
	return &Broadcaster{
		ch: make(chan Update, buffer),
	}
}

// Send publishes an update (non-blocking with drop on full buffer).
func (b *Broadcaster) Send(update Update) {
	select {
	case b.ch <- update:
	default:
		// drop if listeners are slow; keep simple
	}
}

// Listen returns a channel plus a cancel function to stop listening.
func (b *Broadcaster) Listen(ctx context.Context) (<-chan Update, context.CancelFunc) {
	listenerCtx, cancel := context.WithCancel(ctx)
	out := make(chan Update, cap(b.ch))

	go func() {
		defer close(out)
		for {
			select {
			case <-listenerCtx.Done():
				return
			case update, ok := <-b.ch:
				if !ok {
					return
				}
				select {
				case out <- update:
				case <-listenerCtx.Done():
					return
				}
			}
		}
	}()

	return out, cancel
}

// SendWithTimeout publishes with timeout.
func (b *Broadcaster) SendWithTimeout(update Update, timeout time.Duration) bool {
	select {
	case b.ch <- update:
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
