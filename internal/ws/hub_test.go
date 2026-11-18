package ws

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/coder/websocket"
)

func TestDeliveryAndPresence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	changes := make(chan bool, 8)
	h := New(func(_ int64, on bool) { changes <- on })
	srv := httptest.NewServer(h.Handler(func(token string) (Identity, bool) {
		switch token {
		case "admin":
			return Identity{Role: Admin}, true
		case "team":
			return Identity{Team, 1}, true
		}
		return Identity{}, false
	}))
	defer srv.Close()
	defer h.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?token="
	if _, resp, err := websocket.Dial(ctx, url+"bad", nil); err == nil || resp.StatusCode != 401 {
		t.Fatal("invalid token", err)
	}
	dial := func(token string) *websocket.Conn {
		t.Helper()
		c, _, err := websocket.Dial(ctx, url+token, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.CloseNow() })
		return c
	}
	expectPresence := func(want bool) {
		t.Helper()
		select {
		case got := <-changes:
			if got != want {
				t.Fatal(got)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	guest, admin, team1, team2 := dial(""), dial("admin"), dial("team"), dial("team")
	expectPresence(true)
	for {
		h.mu.Lock()
		ready := len(h.clients) == 4
		h.mu.Unlock()
		if ready {
			break
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(time.Millisecond)
	}
	h.ToAdmin(api.WSMessage{Type: "admin"})
	h.ToTeam(2, api.WSMessage{Type: "other"})
	h.ToTeam(1, api.WSMessage{Type: "one"})
	h.ToTeams(api.WSMessage{Type: "teams"})
	h.ToAll(api.WSMessage{Type: "all"})
	for c, types := range map[*websocket.Conn][]string{guest: {"all"}, admin: {"admin", "all"}, team1: {"one", "teams", "all"}, team2: {"one", "teams", "all"}} {
		for _, want := range types {
			_, b, err := c.Read(ctx)
			var m api.WSMessage
			if err != nil || json.Unmarshal(b, &m) != nil || m.Type != want {
				t.Fatal(want, string(b), err)
			}
		}
	}
	team1.CloseNow()
	if !h.Online(1) {
		t.Fatal("team is still connected")
	}
	h.DropTeam(1)
	expectPresence(false)
	if _, _, err := team2.Read(ctx); err == nil {
		t.Fatal("drop failed")
	}
	dial("team")
	expectPresence(true)
	h.Close()
	expectPresence(false)
}

func TestFullQueueDisconnects(t *testing.T) {
	h := New(nil)
	defer h.Close()
	ctx, cancel := context.WithCancel(context.Background())
	c := &client{Identity: Identity{Team, 1}, send: make(chan []byte, 1), cancel: cancel}
	h.clients[c] = true
	h.counts[1] = 1
	h.ToAll(api.WSMessage{})
	h.ToAll(api.WSMessage{})
	if ctx.Err() == nil || h.Online(1) {
		t.Fatal("full queue stayed online")
	}
}
