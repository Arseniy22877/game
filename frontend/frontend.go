package main

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/Arseniy22877/game/gameengine"
)

//go:embed web/index.html web/app.css web/app.js
var webFiles embed.FS

type frontendServer struct {
	mu    sync.Mutex
	game  gameengine.Game
	state gameengine.GameState
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

func newFrontendServer(game gameengine.Game) *frontendServer {
	return &frontendServer{game: game, state: game.CreateInstance()}
}

func (s *frontendServer) snapshot() (frontendState, error) {
	s.state.CalculateFinals()

	result := frontendState{
		Cycle:      s.state.Iteration,
		Money:      s.state.Budget.Money,
		MaxMoney:   s.state.Budget.BudgetData.MaxMoney,
		QP:         s.state.Budget.QP,
		Basic:      make([]frontendAttribute, 0, len(s.game.BasicAttributes)),
		Attributes: make([]frontendAttribute, 0, len(s.game.ConfigurableAttributes)),
		Strategies: make([]frontendStrategy, 0, len(s.game.Strategies)),
		Hints:      make([]frontendHint, 0, len(s.game.Hints)),
		NewEvents:  make([]frontendEvent, 0, len(s.game.Events)),
		History:    make([]frontendEvent, 0, len(s.game.Events)),
	}
	for _, attribute := range s.game.BasicAttributes {
		state := s.state.Attributes[attribute.ID]
		result.Basic = append(result.Basic, frontendAttribute{
			ID: int(attribute.ID), Name: attribute.Name, Description: attribute.Description,
			Value: state.FinalValue, Min: attribute.MinValue, Max: attribute.MaxValue,
		})
	}
	for _, attribute := range s.game.ConfigurableAttributes {
		state := s.state.Attributes[attribute.ID]
		result.Attributes = append(result.Attributes, frontendAttribute{
			ID: int(attribute.ID), Name: attribute.Name, Description: attribute.Description,
			Value: state.BaseValue, Min: attribute.MinValue, Max: attribute.MaxValue,
			CostPerUnit: attribute.CostPerUnit,
		})
	}
	for _, strategy := range s.game.Strategies {
		state := s.state.Strategies[strategy.ID]
		if state.Status == gameengine.StrategyLockedStatus {
			continue
		}
		result.Strategies = append(result.Strategies, frontendStrategy{
			ID: int(strategy.ID), Name: strategy.Name, Description: strategy.Description,
			Cost: strategy.Cost, Unlocked: true,
			Active: state.Status == gameengine.StrategyUsingStatus,
		})
	}
	for _, hint := range s.game.Hints {
		state := s.state.Hints[hint.ID]
		if state.Status == gameengine.HintLockedStatus {
			continue
		}
		result.Hints = append(result.Hints, frontendHint{
			ID: int(hint.ID), Name: hint.Name, Description: hint.Description,
			Reliability: hint.Reliability, Cost: hint.Cost,
			Unlocked: true, Used: state.Status == gameengine.HintUsedStatus,
			Secret: func() string {
				if state.Status == gameengine.HintUsedStatus {
					return hint.Secret
				}
				return ""
			}(),
		})
	}
	for _, event := range s.game.Events {
		state := s.state.Events[event.ID]
		if state.BaseStatus == gameengine.EventFiredStatus {
			result.NewEvents = append(result.NewEvents, frontendEvent{
				ID: int(event.ID), Name: event.Name, Description: event.Description,
				Positive: event.IsPositive, Status: "fired",
			})
			continue
		}
		if state.BaseStatus == gameengine.EventExtinguishedStatus {
			result.History = append(result.History, frontendEvent{
				ID: int(event.ID), Name: event.Name, Description: event.Description,
				Positive: event.IsPositive, Status: "extinguished",
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
	attributeIDs := make([]int, 0, len(request.Attributes))
	attributeValues := make([]float64, 0, len(request.Attributes))
	for _, attribute := range request.Attributes {
		attributeIDs = append(attributeIDs, attribute.ID)
		attributeValues = append(attributeValues, attribute.Value)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.state.PassAtributes(attributeIDs, attributeValues); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.state.PassStrategies(request.Strategies); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.state.PassHints(request.Hints); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.state.CalculateState(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.state.FireEvents(nil); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.state.Iteration++
	s.writeState(w)
}

func (s *frontendServer) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	s.state = s.game.CreateInstance()
	s.mu.Unlock()
	s.writeState(w)
}

func serveFrontend(addr string, game gameengine.Game) error {
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
