package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/coder/websocket"
)

type Role string

const (
	Guest Role = "guest"
	Admin Role = "admin"
	Team  Role = "team"
)

type Identity struct {
	Role   Role
	TeamID int64
}

type client struct {
	Identity
	send   chan []byte
	cancel context.CancelFunc
}

type presence struct {
	id     int64
	online bool
}

type Hub struct {
	mu         sync.Mutex
	clients    map[*client]bool // false while a replaced team's queued notification is draining
	counts     map[int64]int
	closed     bool
	generation uint64
	wg         sync.WaitGroup
	wake       *sync.Cond
	changes    []presence
	onPresence func(int64, bool)
}

func New(onPresence func(int64, bool)) *Hub {
	h := &Hub{clients: map[*client]bool{}, counts: map[int64]int{}, onPresence: onPresence}
	h.wake = sync.NewCond(&h.mu)
	h.wg.Add(1)
	go h.presenceLoop()
	return h
}

// Deliver transitions in order without holding the hub lock across manager calls.
func (h *Hub) presenceLoop() {
	defer h.wg.Done()
	h.mu.Lock()
	defer h.mu.Unlock()
	for {
		for len(h.changes) == 0 && !h.closed {
			h.wake.Wait()
		}
		if len(h.changes) == 0 {
			return
		}
		batch := h.changes
		h.changes = nil
		h.mu.Unlock()
		for _, p := range batch {
			h.onPresence(p.id, p.online)
		}
		h.mu.Lock()
	}
}

func (h *Hub) changed(id int64, online bool) {
	if h.onPresence != nil {
		h.changes = append(h.changes, presence{id, online})
		h.wake.Signal()
	}
}

func (h *Hub) detach(c *client) {
	if !h.clients[c] {
		return
	}
	h.clients[c] = false
	if c.Role == Team {
		h.counts[c.TeamID]--
		if h.counts[c.TeamID] == 0 {
			delete(h.counts, c.TeamID)
			h.changed(c.TeamID, false)
		}
	}
}

func (h *Hub) remove(c *client) {
	h.detach(c)
	delete(h.clients, c)
	c.cancel()
}

func (h *Hub) Online(id int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.counts[id] > 0
}
func (h *Hub) DropTeam(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.generation++
	for c := range h.clients {
		if c.Role == Team && c.TeamID == id {
			h.remove(c)
		}
	}
}

func (h *Hub) DropAllTeams() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.generation++
	for c, active := range h.clients {
		if active && c.Role == Team {
			h.detach(c)
			// The writer closes the connection after sending the queued phase change.
			close(c.send)
		}
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	for c := range h.clients {
		h.remove(c)
	}
	h.wake.Broadcast()
	h.mu.Unlock()
	h.wg.Wait()
}

func (h *Hub) broadcast(role Role, id int64, m api.WSMessage) {
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c, active := range h.clients {
		if !active || role != "" && c.Role != role || id != 0 && c.TeamID != id {
			continue
		}
		select {
		case c.send <- data:
		default:
			h.remove(c)
		}
	}
}

func (h *Hub) ToAll(m api.WSMessage)   { h.broadcast("", 0, m) }
func (h *Hub) ToAdmin(m api.WSMessage) { h.broadcast(Admin, 0, m) }
func (h *Hub) ToTeams(m api.WSMessage) { h.broadcast(Team, 0, m) }
func (h *Hub) ToTeam(id int64, m api.WSMessage) {
	if id > 0 {
		h.broadcast(Team, id, m)
	}
}

func (h *Hub) Handler(resolve func(string) (Identity, bool)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		// A team can be archived while token resolution or the handshake is in flight.
		generation := h.generation
		h.mu.Unlock()
		identity := Identity{Role: Guest}
		if token := r.URL.Query().Get("token"); token != "" {
			var ok bool
			identity, ok = resolve(token)
			if !ok {
				http.Error(w, "unauthorized", 401)
				return
			}
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		c := &client{Identity: identity, send: make(chan []byte, 16), cancel: cancel}
		h.mu.Lock()
		if h.closed || identity.Role == Team && generation != h.generation {
			h.mu.Unlock()
			return
		}
		h.wg.Add(1)
		defer h.wg.Done()
		h.clients[c] = true
		if c.Role == Team {
			h.counts[c.TeamID]++
			if h.counts[c.TeamID] == 1 {
				h.changed(c.TeamID, true)
			}
		}
		h.mu.Unlock()
		defer func() { h.mu.Lock(); h.remove(c); h.mu.Unlock() }()
		conn.SetReadLimit(1024)
		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			defer cancel()
			for {
				if _, _, err := conn.Read(ctx); err != nil {
					return
				}
			}
		}()
		defer func() { cancel(); conn.CloseNow(); <-readDone }()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-c.send:
				if !ok {
					return
				}
				deadline, stop := context.WithTimeout(ctx, 5*time.Second)
				err = conn.Write(deadline, websocket.MessageText, data)
				stop()
			case <-ticker.C:
				deadline, stop := context.WithTimeout(ctx, 5*time.Second)
				err = conn.Ping(deadline)
				stop()
			}
			if err != nil {
				return
			}
		}
	})
}
