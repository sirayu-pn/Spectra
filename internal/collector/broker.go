package collector

import (
	"sync"
)

// Broker handles broadcasting real-time system metrics to SSE subscribers.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[chan []byte]struct{}
}

// NewBroker creates an initialized SSE broker.
func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[chan []byte]struct{}),
	}
}

// Subscribe adds a new client channel and returns an unsubscribe cleanup closure.
func (b *Broker) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 16)

	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		delete(b.subscribers, ch)
		b.mu.Unlock()
	}

	return ch, unsubscribe
}

// Broadcast sends a message slice to all active subscribers.
// Non-blocking write: if a subscriber's buffer is full, drop frame to prevent lagging others.
func (b *Broker) Broadcast(data []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.subscribers {
		select {
		case ch <- data:
		default:
			// Slow consumer, skip frame
		}
	}
}

// SubscriberCount returns the current count of connected SSE subscribers.
func (b *Broker) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}
