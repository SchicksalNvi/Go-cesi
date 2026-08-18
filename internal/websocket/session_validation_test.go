package websocket

import (
	"testing"
	"time"

	"superview/internal/auth"
	"superview/internal/supervisor"
)

func TestCheckHeartbeatsDisconnectsRevokedSession(t *testing.T) {
	hub := NewHub(supervisor.NewSupervisorService())
	hub.unregister = make(chan *Client, 1)
	hub.SetSessionValidator(func(userID string, tokenVersion uint64) error {
		if userID != "revoked-user" || tokenVersion != 7 {
			t.Fatalf("unexpected session identity: %s/%d", userID, tokenVersion)
		}
		return auth.ErrSessionUnavailable
	})

	client := &Client{
		hub:          hub,
		userID:       "revoked-user",
		tokenVersion: 7,
		lastPong:     time.Now(),
	}
	hub.clients[client] = true

	hub.checkHeartbeats()

	select {
	case disconnected := <-hub.unregister:
		if disconnected != client {
			t.Fatal("unexpected client disconnected")
		}
	case <-time.After(time.Second):
		t.Fatal("revoked WebSocket session was not disconnected")
	}

	client.mu.RLock()
	closed := client.closed
	client.mu.RUnlock()
	if !closed {
		t.Fatal("revoked WebSocket client was not marked closed")
	}
}
