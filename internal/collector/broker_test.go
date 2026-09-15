package collector

import (
	"context"
	"testing"
	"time"
)

func TestBrokerSubscribeBroadcast(t *testing.T) {
	broker := NewBroker()

	if count := broker.SubscriberCount(); count != 0 {
		t.Fatalf("expected 0 subscribers, got %d", count)
	}

	ch, unsubscribe := broker.Subscribe()
	if count := broker.SubscriberCount(); count != 1 {
		t.Fatalf("expected 1 subscriber, got %d", count)
	}

	testPayload := []byte(`{"test":"sse"}`)
	broker.Broadcast(testPayload)

	select {
	case msg := <-ch:
		if string(msg) != string(testPayload) {
			t.Fatalf("expected %s, got %s", string(testPayload), string(msg))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for broadcast message")
	}

	unsubscribe()
	if count := broker.SubscriberCount(); count != 0 {
		t.Fatalf("expected 0 subscribers after unsubscribe, got %d", count)
	}
}

func TestCollectorBackgroundTicker(t *testing.T) {
	col := NewCollector()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	col.Start(ctx, 50*time.Millisecond)

	// Verify immediate initial snapshot availability
	time.Sleep(20 * time.Millisecond)
	snap := col.GetLatestSnapshot()
	if snap == nil {
		t.Fatal("expected non-nil latest snapshot")
	}

	jsonData := col.GetLatestJSON()
	if len(jsonData) == 0 {
		t.Fatal("expected non-empty JSON data")
	}

	// Verify broker received message
	ch, unsubscribe := col.Broker().Subscribe()
	defer unsubscribe()

	select {
	case msg := <-ch:
		if len(msg) == 0 {
			t.Fatal("received empty message from broker")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for background ticker broadcast")
	}
}
