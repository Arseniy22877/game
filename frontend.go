package main

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"sync"
)

//go:embed web/index.html web/app.css web/app.js
var webFiles embed.FS

type frontendServer struct {
	mu    sync.Mutex
	game  Game
	state GameState
}

type frontendAttribute struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Value       float64 `json:"value"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	CostPerUnit float64 `json:"costPerUnit"`
}

type frontendStrategy struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Cost        int    `json:"cost"`
	Unlocked    bool   `json:"unlocked"`
	Active      bool   `json:"active"`
}

type frontendHint struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Reliability string `json:"reliability"`
	Cost        int    `json:"cost"`
	Unlocked    bool   `json:"unlocked"`
	Used        bool   `json:"used"`
	Secret      string `json:"secret,omitempty"`
}

type frontendEvent struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Positive    bool   `json:"positive"`
	Status      string `json:"status"`
}

type frontendState struct {
	Cycle      int                 `json:"cycle"`
	Money      int                 `json:"money"`
	MaxMoney   int                 `json:"maxMoney"`
	QP         int                 `json:"qp"`
	Basic      []frontendAttribute `json:"basicAttributes"`
	Attributes []frontendAttribute `json:"attributes"`
	Strategies []frontendStrategy  `json:"strategies"`
	Hints      []frontendHint      `json:"hints"`
	NewEvents  []frontendEvent     `json:"newEvents"`
	History    []frontendEvent     `json:"history"`
}

type cycleRequest struct {
	Attributes []struct {
		ID    int     `json:"id"`
		Value float64 `json:"value"`
	} `json:"attributes"`
	Strategies []int `json:"strategies"`
	Hints      []int `json:"hints"`
	Cost       int   `json:"cost"`
}

func newFrontendServer(game Game) *frontendServer {
	return &frontendServer{game: game, state: game.createInstance()}
}

func (s *frontendServer) snapshot() (frontendState, error) {
	s.state.calcFinals()

	result := frontendState{
		Cycle:      s.state.iteration,
		Money:      s.state.budget.money,
		MaxMoney:   s.state.budget._budgetData.maxMoney,
		QP:         s.state.budget.qp,
		Basic:      make([]frontendAttribute, 0, len(s.game.basicAttributes)),
		Attributes: make([]frontendAttribute, 0, len(s.game.configurableAttributes)),
		Strategies: make([]frontendStrategy, 0, len(s.game.strategies)),
		Hints:      make([]frontendHint, 0, len(s.game.hints)),
		NewEvents:  make([]frontendEvent, 0, len(s.game.events)),
		History:    make([]frontendEvent, 0, len(s.game.events)),
	}
	for _, attribute := range s.game.basicAttributes {
		state := s.state.attributes[attribute.id]
		result.Basic = append(result.Basic, frontendAttribute{
			ID: int(attribute.id), Name: attribute.name, Description: attribute.description,
			Value: state.finalValue, Min: attribute.minValue, Max: attribute.maxValue,
		})
	}
	for _, attribute := range s.game.configurableAttributes {
		state := s.state.attributes[attribute.id]
		result.Attributes = append(result.Attributes, frontendAttribute{
			ID: int(attribute.id), Name: attribute.name, Description: attribute.description,
			Value: state.baseValue, Min: attribute.minValue, Max: attribute.maxValue,
			CostPerUnit: attribute.costPerUnit,
		})
	}
	for _, strategy := range s.game.strategies {
		state := s.state.strategies[strategy.id]
		if state.status == StrategyLockedStatus {
			continue
		}
		result.Strategies = append(result.Strategies, frontendStrategy{
			ID: int(strategy.id), Name: strategy.name, Description: strategy.description,
			Cost: strategy.cost, Unlocked: true,
			Active: state.status == StrategyUsingStatus,
		})
	}
	for _, hint := range s.game.hints {
		state := s.state.hints[hint.id]
		if state.status == HintLockedStatus {
			continue
		}
		result.Hints = append(result.Hints, frontendHint{
			ID: int(hint.id), Name: hint.name, Description: hint.description,
			Reliability: hint.reliability, Cost: hint.cost,
			Unlocked: true, Used: state.status == HintUsedStatus,
			Secret: func() string {
				if state.status == HintUsedStatus {
					return hint.secret
				}
				return ""
			}(),
		})
	}
	for _, event := range s.game.events {
		state := s.state.events[event.id]
		if state.baseStatus == EventFiredStatus {
			result.NewEvents = append(result.NewEvents, frontendEvent{
				ID: int(event.id), Name: event.name, Description: event.description,
				Positive: event.isPositive, Status: "fired",
			})
			continue
		}
		if state.baseStatus == EventExtinguishedStatus {
			result.History = append(result.History, frontendEvent{
				ID: int(event.id), Name: event.name, Description: event.description,
				Positive: event.isPositive, Status: "extinguished",
			})
		}
	}
	return result, nil
}

func (s *frontendServer) writeState(w http.ResponseWriter) {
	state, err := s.snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

func (s *frontendServer) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeState(w)
}

func (s *frontendServer) handleCycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request cycleRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid cycle request: "+err.Error(), http.StatusBadRequest)
		return
	}
	changes := make([]AttributeChange, 0, len(request.Attributes))
	for _, attribute := range request.Attributes {
		changes = append(changes, AttributeChange{attributeId: AttributeId(attribute.ID), attributeShift: attribute.Value})
	}
	strategies := make([]StrategyId, 0, len(request.Strategies))
	for _, id := range request.Strategies {
		strategies = append(strategies, StrategyId(id))
	}
	hints := make([]HintId, 0, len(request.Hints))
	for _, id := range request.Hints {
		hints = append(hints, HintId(id))
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.state.pushCycle(changes, strategies, hints, BudgetChange{moneyShift: -request.Cost}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.state.iteration++
	s.writeState(w)
}

func (s *frontendServer) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	s.state = s.game.createInstance()
	s.mu.Unlock()
	s.writeState(w)
}

func serveFrontend(addr string, game Game) error {
	server := newFrontendServer(game)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", server.handleState)
	mux.HandleFunc("/api/cycle", server.handleCycle)
	mux.HandleFunc("/api/reset", server.handleReset)
	staticFiles := http.FileServer(http.FS(webFiles))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/web/index.html", http.StatusFound)
			return
		}
		staticFiles.ServeHTTP(w, r)
	})
	log.Printf("game frontend is available at http://%s", addr)
	return http.ListenAndServe(addr, mux)
}
