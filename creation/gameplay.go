package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Arseniy22877/game/gameengine"
)

const (
	playSessionTTL  = 12 * time.Hour
	maxPlayBodySize = 256 << 10
)

type playSession struct {
	mu        sync.Mutex
	state     gameengine.GameState
	scenario  string
	lastTurn  *playTurnResult
	updatedAt time.Time
}

var playSessionStore = struct {
	sync.RWMutex
	items map[string]*playSession
}{items: make(map[string]*playSession)}

type playStartRequest struct {
	Scenario string `json:"scenario"`
}

type playAttributeInput struct {
	ID    int     `json:"id"`
	Value float64 `json:"value"`
}

type playTurnRequest struct {
	SessionID  string               `json:"sessionId"`
	Attributes []playAttributeInput `json:"attributes"`
	Strategies []int                `json:"strategies"`
	Hints      []int                `json:"hints"`
}

type playAttributeView struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Min          float64 `json:"min"`
	Max          float64 `json:"max"`
	BaseValue    float64 `json:"baseValue"`
	FinalValue   float64 `json:"finalValue"`
	Configurable bool    `json:"configurable"`
	CostPerUnit  float64 `json:"costPerUnit,omitempty"`
}

type playPrerequisiteView struct {
	Events     []int `json:"events"`
	Strategies []int `json:"strategies"`
}

type playStrategyView struct {
	ID            int                  `json:"id"`
	Name          string               `json:"name"`
	Description   string               `json:"description"`
	Cost          int                  `json:"cost"`
	Active        bool                 `json:"active"`
	Prerequisites playPrerequisiteView `json:"prerequisites"`
}

type playHintView struct {
	ID            int                  `json:"id"`
	Name          string               `json:"name"`
	Description   string               `json:"description"`
	Secret        string               `json:"secret,omitempty"`
	Reliability   string               `json:"reliability"`
	Cost          int                  `json:"cost"`
	Used          bool                 `json:"used"`
	Available     bool                 `json:"available"`
	Prerequisites playPrerequisiteView `json:"prerequisites"`
}

type playEventView struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	ImageURL         string `json:"imageURL"`
	Branch           string `json:"branch"`
	Iteration        int    `json:"iteration"`
	Positive         bool   `json:"positive"`
	Status           string `json:"status"`
	MoneyShift       int    `json:"moneyShift"`
	QPShift          int    `json:"qpShift"`
	AttributeChanges []struct {
		AttributeID int     `json:"attributeID"`
		Name        string  `json:"name"`
		Shift       float64 `json:"shift"`
	} `json:"attributeChanges"`
}

type playEventPrediction struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	Share  float64 `json:"share"`
}

type playBranchPrediction struct {
	Branch   string                `json:"branch"`
	Sum      float64               `json:"sum"`
	Eligible bool                  `json:"eligible"`
	Events   []playEventPrediction `json:"events"`
}

type playTurnResult struct {
	Turn        int                    `json:"turn"`
	Predictions []playBranchPrediction `json:"predictions"`
	Events      []playEventView        `json:"events"`
	NoEvents    bool                   `json:"noEvents"`
}

type playSnapshot struct {
	SessionID         string              `json:"sessionId"`
	Scenario          string              `json:"scenario"`
	Iteration         int                 `json:"iteration"`
	Money             int                 `json:"money"`
	MaxMoney          int                 `json:"maxMoney"`
	QP                int                 `json:"qp"`
	MinProbabilitySum float64             `json:"minProbabilitySum"`
	Attributes        []playAttributeView `json:"attributes"`
	Strategies        []playStrategyView  `json:"strategies"`
	Hints             []playHintView      `json:"hints"`
	Events            []playEventView     `json:"events"`
	LastTurn          *playTurnResult     `json:"lastTurn,omitempty"`
}

// registerGameplayRoutes connects the browser game page to gameengine's API.
// Call it once after creating the application's http.ServeMux.
func registerGameplayRoutes(mux *http.ServeMux) error {
	if mux == nil {
		return errors.New("gameplay: nil HTTP mux")
	}
	mux.HandleFunc("/play", handlePlayPage)
	mux.HandleFunc("/api/play/scenarios", handlePlayScenarios)
	mux.HandleFunc("/api/play/start", handlePlayStart)
	mux.HandleFunc("/api/play/state", handlePlayState)
	mux.HandleFunc("/api/play/turn", handlePlayTurn)
	return nil
}

func handlePlayPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/play" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(gameplayHTML))
}

func handlePlayScenarios(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		playWriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	entries, err := os.ReadDir(scenariosDirectory)
	if err != nil {
		playWriteError(w, http.StatusInternalServerError, "cannot read scenarios: "+err.Error())
		return
	}

	type scenarioOption struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	result := make([]scenarioOption, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if _, err := scenarioPath(id); err != nil {
			continue
		}
		if _, err := gameengine.CreateGameJSON(filepath.Join(scenariosDirectory, entry.Name())); err != nil {
			continue
		}
		result = append(result, scenarioOption{ID: id, Title: id})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Title < result[j].Title })
	playWriteJSON(w, result)
}

func handlePlayStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		playWriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var request playStartRequest
	if err := playDecodeJSON(w, r, &request); err != nil {
		playWriteError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	path, err := scenarioPath(request.Scenario)
	if err != nil {
		playWriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	scenarioFile, err := gameengine.CreateGameJSON(path)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		playWriteError(w, status, err.Error())
		return
	}
	registeredScenario := scenarioFile
	gameengine.RegisterGame(request.Scenario, func() gameengine.Game { return registeredScenario })
	state, err := gameengine.GetGameInstance(request.Scenario)
	if err != nil {
		playWriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if state.GameData == nil {
		playWriteError(w, http.StatusInternalServerError, "scenario produced an empty game state")
		return
	}

	// Iteration 0 is reserved for events that are not tied to a turn.
	// The first player-controlled cycle is therefore iteration 1.
	state.Iteration = 1
	sessionID, err := newPlaySessionID()
	if err != nil {
		playWriteError(w, http.StatusInternalServerError, "cannot create game session")
		return
	}

	session := &playSession{
		state:     state,
		scenario:  request.Scenario,
		updatedAt: time.Now(),
	}
	playSessionStore.Lock()
	prunePlaySessionsLocked(time.Now())
	playSessionStore.items[sessionID] = session
	playSessionStore.Unlock()

	playWriteJSON(w, buildPlaySnapshot(sessionID, session))
}

func handlePlayState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		playWriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
	session := getPlaySession(sessionID)
	if session == nil {
		playWriteError(w, http.StatusNotFound, "game session not found or expired")
		return
	}

	session.mu.Lock()
	session.updatedAt = time.Now()
	snapshot := buildPlaySnapshot(sessionID, session)
	session.mu.Unlock()
	playWriteJSON(w, snapshot)
}

func handlePlayTurn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		playWriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var request playTurnRequest
	if err := playDecodeJSON(w, r, &request); err != nil {
		playWriteError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	session := getPlaySession(strings.TrimSpace(request.SessionID))
	if session == nil {
		playWriteError(w, http.StatusNotFound, "game session not found or expired")
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	next := clonePlayState(session.state)
	turn := next.Iteration
	attributeIDs := make([]int, len(request.Attributes))
	attributeValues := make([]float64, len(request.Attributes))
	for i, attribute := range request.Attributes {
		attributeIDs[i] = attribute.ID
		attributeValues[i] = attribute.Value
	}

	if err := next.PassAttributes(attributeIDs, attributeValues); err != nil {
		playWriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := next.PassStrategies(request.Strategies); err != nil {
		playWriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := next.PassHints(request.Hints); err != nil {
		playWriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := next.CalculateState(); err != nil {
		playWriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	predictions, err := next.GetPredicts()
	if err != nil {
		playWriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	branchPredictions := buildPlayPredictions(next.GameData.Branches, predictions, next.GameData.MinProbabilitySum)

	// This game page deliberately has no event picker: all branches are resolved
	// by the engine's automatic picker through FireEvents(nil).
	fireErr := next.FireEvents(nil)
	noEvents := false
	if fireErr != nil {
		if isPlayNoEventsError(fireErr) {
			noEvents = true
			// FireEvents returns before status transitions when nothing was picked.
			// Retire the previous turn's Fired marker as the cycle still advances.
			for id, event := range next.Events {
				if event.Status == gameengine.EventFiredStatus {
					event.Status = gameengine.EventExtinguishedStatus
					next.Events[id] = event
				}
			}
		} else {
			playWriteError(w, http.StatusUnprocessableEntity, fireErr.Error())
			return
		}
	}

	firedEvents := make([]playEventView, 0)
	for _, event := range next.Events {
		if event.Status == gameengine.EventFiredStatus && event.EventData != nil {
			firedEvents = append(firedEvents, makePlayEventView(next, event, "fired"))
		}
	}
	sort.Slice(firedEvents, func(i, j int) bool { return firedEvents[i].ID < firedEvents[j].ID })

	next.Iteration++
	session.state = next
	session.lastTurn = &playTurnResult{
		Turn:        turn,
		Predictions: branchPredictions,
		Events:      firedEvents,
		NoEvents:    noEvents || len(firedEvents) == 0,
	}
	session.updatedAt = time.Now()
	playWriteJSON(w, buildPlaySnapshot(request.SessionID, session))
}

func buildPlaySnapshot(sessionID string, session *playSession) playSnapshot {
	state := session.state
	result := playSnapshot{
		SessionID:         sessionID,
		Scenario:          session.scenario,
		Iteration:         state.Iteration,
		Money:             state.Budget.Money,
		QP:                state.Budget.QP,
		MinProbabilitySum: 0,
		Attributes:        make([]playAttributeView, 0, len(state.Attributes)),
		Strategies:        make([]playStrategyView, 0, len(state.Strategies)),
		Hints:             make([]playHintView, 0, len(state.Hints)),
		Events:            make([]playEventView, 0, len(state.Events)),
		LastTurn:          session.lastTurn,
	}
	if state.GameData != nil {
		result.MinProbabilitySum = state.GameData.MinProbabilitySum
		if state.Budget.BudgetData != nil {
			result.MaxMoney = state.Budget.BudgetData.MaxMoney
		}
	}

	configurable := make(map[gameengine.AttributeId]float64)
	if state.GameData != nil {
		for _, attribute := range state.GameData.ConfigurableAttributes {
			configurable[attribute.ID] = attribute.CostPerUnit
		}
	}
	attributeIDs := make([]int, 0, len(state.Attributes))
	for id := range state.Attributes {
		attributeIDs = append(attributeIDs, int(id))
	}
	sort.Ints(attributeIDs)
	for _, rawID := range attributeIDs {
		attribute := state.Attributes[gameengine.AttributeId(rawID)]
		if attribute.AttributeData == nil {
			continue
		}
		cost, isConfigurable := configurable[gameengine.AttributeId(rawID)]
		result.Attributes = append(result.Attributes, playAttributeView{
			ID:           rawID,
			Name:         attribute.AttributeData.Name,
			Description:  attribute.AttributeData.Description,
			Min:          attribute.AttributeData.MinValue,
			Max:          attribute.AttributeData.MaxValue,
			BaseValue:    attribute.BaseValue,
			FinalValue:   attribute.FinalValue,
			Configurable: isConfigurable,
			CostPerUnit:  cost,
		})
	}

	// Only strategies unlocked in the current state are sent to the client.
	for _, strategy := range state.GetAvailableStrategies() {
		if strategy.StrategyData == nil {
			continue
		}
		result.Strategies = append(result.Strategies, playStrategyView{
			ID:            int(strategy.StrategyData.ID),
			Name:          strategy.StrategyData.Name,
			Description:   strategy.StrategyData.Description,
			Cost:          strategy.StrategyData.Cost,
			Active:        strategy.Status == gameengine.StrategyUsingStatus,
			Prerequisites: makePlayPrerequisiteView(strategy.StrategyData.Prerequisites),
		})
	}

	// Available hints are filtered through the engine API. Previously used
	// hints stay visible so that their Secret remains available to the player.
	availableHintIDs := make(map[gameengine.HintId]struct{})
	for _, hint := range state.GetAvailableHints() {
		if hint.HintData != nil {
			availableHintIDs[hint.HintData.ID] = struct{}{}
		}
	}
	hintIDs := make([]int, 0, len(state.Hints))
	for id := range state.Hints {
		hintIDs = append(hintIDs, int(id))
	}
	sort.Ints(hintIDs)
	for _, rawID := range hintIDs {
		hint := state.Hints[gameengine.HintId(rawID)]
		if hint.HintData == nil {
			continue
		}
		used := hint.Status == gameengine.HintUsedStatus
		if !used {
			if _, available := availableHintIDs[gameengine.HintId(rawID)]; !available {
				continue
			}
		}
		secret := ""
		if used {
			secret = hint.HintData.Secret
		}
		result.Hints = append(result.Hints, playHintView{
			ID:            rawID,
			Name:          hint.HintData.Name,
			Description:   hint.HintData.Description,
			Secret:        secret,
			Reliability:   hint.HintData.Reliability,
			Cost:          hint.HintData.Cost,
			Used:          used,
			Available:     !used,
			Prerequisites: makePlayPrerequisiteView(hint.HintData.Prerequisites),
		})
	}

	eventIDs := make([]int, 0, len(state.Events))
	for id := range state.Events {
		eventIDs = append(eventIDs, int(id))
	}
	sort.Ints(eventIDs)
	for _, rawID := range eventIDs {
		event := state.Events[gameengine.EventId(rawID)]
		if event.EventData == nil {
			continue
		}
		status := "neutral"
		switch event.Status {
		case gameengine.EventFiredStatus:
			status = "fired"
		case gameengine.EventExtinguishedStatus:
			status = "extinguished"
		}
		result.Events = append(result.Events, makePlayEventView(state, event, status))
	}
	return result
}

func makePlayEventView(state gameengine.GameState, event gameengine.EventState, status string) playEventView {
	data := event.EventData
	view := playEventView{
		ID:          int(data.ID),
		Name:        data.Name,
		Description: data.Description,
		ImageURL:    data.ImageURL,
		Branch:      string(data.Branch),
		Iteration:   data.Iteration,
		Positive:    data.IsPositive,
		Status:      status,
		MoneyShift:  data.BudgetChange.MoneyShift,
		QPShift:     data.BudgetChange.QPShift,
	}
	for _, change := range data.AttributeChanges {
		name := fmt.Sprintf("Атрибут %d", change.AttributeID)
		if attribute, ok := state.Attributes[change.AttributeID]; ok && attribute.AttributeData != nil {
			name = attribute.AttributeData.Name
		}
		view.AttributeChanges = append(view.AttributeChanges, struct {
			AttributeID int     `json:"attributeID"`
			Name        string  `json:"name"`
			Shift       float64 `json:"shift"`
		}{AttributeID: int(change.AttributeID), Name: name, Shift: change.AttributeShift})
	}
	return view
}

func buildPlayPredictions(branches []gameengine.Branch, probabilities []gameengine.ProbResult, minSum float64) []playBranchPrediction {
	byBranch := make(map[gameengine.Branch][]gameengine.ProbResult, len(branches))
	for _, probability := range probabilities {
		if probability.Event == nil || probability.Event.EventData == nil || probability.Probability <= 0 {
			continue
		}
		branch := probability.Event.EventData.Branch
		byBranch[branch] = append(byBranch[branch], probability)
	}

	result := make([]playBranchPrediction, 0, len(branches))
	for _, branch := range branches {
		items := byBranch[branch]
		var sum float64
		for _, item := range items {
			sum += item.Probability
		}
		prediction := playBranchPrediction{
			Branch:   string(branch),
			Sum:      sum,
			Eligible: len(items) > 0 && sum >= minSum,
			Events:   make([]playEventPrediction, 0, len(items)),
		}
		if sum > 0 && !math.IsInf(sum, 0) && !math.IsNaN(sum) {
			for _, item := range items {
				prediction.Events = append(prediction.Events, playEventPrediction{
					ID:     int(item.Event.EventData.ID),
					Name:   item.Event.EventData.Name,
					Weight: item.Probability,
					Share:  item.Probability / sum * 100,
				})
			}
			sort.Slice(prediction.Events, func(i, j int) bool {
				return prediction.Events[i].Share > prediction.Events[j].Share
			})
		}
		result = append(result, prediction)
	}
	return result
}

func makePlayPrerequisiteView(prerequisites gameengine.Prerequisite) playPrerequisiteView {
	events := make([]int, len(prerequisites.Events))
	for i, id := range prerequisites.Events {
		events[i] = int(id)
	}
	strategies := make([]int, len(prerequisites.Strategies))
	for i, id := range prerequisites.Strategies {
		strategies[i] = int(id)
	}
	return playPrerequisiteView{Events: events, Strategies: strategies}
}

func clonePlayState(source gameengine.GameState) gameengine.GameState {
	result := source
	result.Attributes = make(map[gameengine.AttributeId]gameengine.AttributeState, len(source.Attributes))
	for id, value := range source.Attributes {
		result.Attributes[id] = value
	}
	result.Events = make(map[gameengine.EventId]gameengine.EventState, len(source.Events))
	for id, value := range source.Events {
		result.Events[id] = value
	}
	result.Hints = make(map[gameengine.HintId]gameengine.HintState, len(source.Hints))
	for id, value := range source.Hints {
		result.Hints[id] = value
	}
	result.Strategies = make(map[gameengine.StrategyId]gameengine.StrategyState, len(source.Strategies))
	for id, value := range source.Strategies {
		result.Strategies[id] = value
	}
	result.PendingAttributes = append([]gameengine.AttributeChange(nil), source.PendingAttributes...)
	result.PendingStrategies = append([]gameengine.StrategyId(nil), source.PendingStrategies...)
	result.PendingHints = append([]gameengine.HintId(nil), source.PendingHints...)
	return result
}

func getPlaySession(id string) *playSession {
	if id == "" {
		return nil
	}
	playSessionStore.RLock()
	session := playSessionStore.items[id]
	playSessionStore.RUnlock()
	if session == nil {
		return nil
	}
	session.mu.Lock()
	expired := time.Since(session.updatedAt) > playSessionTTL
	session.mu.Unlock()
	if expired {
		playSessionStore.Lock()
		if current := playSessionStore.items[id]; current == session {
			delete(playSessionStore.items, id)
		}
		playSessionStore.Unlock()
		return nil
	}
	return session
}

func prunePlaySessionsLocked(now time.Time) {
	for id, session := range playSessionStore.items {
		session.mu.Lock()
		expired := now.Sub(session.updatedAt) > playSessionTTL
		session.mu.Unlock()
		if expired {
			delete(playSessionStore.items, id)
		}
	}
}

func newPlaySessionID() (string, error) {
	data := make([]byte, 24)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func playDecodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxPlayBodySize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func isPlayNoEventsError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no probable events") || strings.Contains(message, "no events for branch")
}

func playWriteJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "cannot encode response", http.StatusInternalServerError)
	}
}

func playWriteError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

const gameplayHTML = `<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Симуляция государства</title>
<style>
:root{color-scheme:dark;--bg:#101820;--panel:#182632;--panel2:#1e303e;--line:#304656;--text:#e8eef2;--muted:#a7b7c3;--accent:#78b9d8;--good:#77c69a;--bad:#e28c83;--gold:#e3c77b;font:15px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text)}button,input,select{font:inherit}button{border:1px solid #46718a;border-radius:8px;padding:9px 14px;background:#28536b;color:#fff;cursor:pointer}button:hover{filter:brightness(1.12)}button:disabled{opacity:.45;cursor:not-allowed}.secondary{background:#293b48;border-color:#455d6c}.danger{background:#583b3a;border-color:#80504c}input,select{width:100%;min-width:0;border:1px solid var(--line);border-radius:7px;padding:9px;background:#101b24;color:var(--text)}input[type=checkbox]{width:auto;accent-color:var(--accent);transform:scale(1.08)}input[type=range]{width:100%;padding:0;border:0;background:transparent;accent-color:var(--accent);cursor:pointer}
header{position:sticky;top:0;z-index:5;background:#111d27ee;backdrop-filter:blur(12px);border-bottom:1px solid var(--line);padding:12px max(18px,calc((100vw - 1440px)/2));display:flex;gap:14px;align-items:center;flex-wrap:wrap}h1{font-size:20px;margin:0;font-weight:700}.toolbar{display:flex;gap:8px;align-items:center;flex-wrap:wrap;flex:1}.toolbar select{max-width:340px}.status{font-size:13px;color:var(--muted);margin-left:auto}.status.error{color:var(--bad)}.status.good{color:var(--good)}main{max-width:1440px;margin:22px auto;padding:0 18px 60px}.intro{color:var(--muted);margin:0 0 18px}.hidden{display:none!important}.grid{display:grid;grid-template-columns:minmax(300px,390px) minmax(0,1fr);gap:16px;align-items:start}.stack{display:grid;gap:14px}.panel{background:var(--panel);border:1px solid var(--line);border-radius:13px;padding:16px;box-shadow:0 8px 25px #0002}.panel h2{font-size:17px;margin:0 0 12px}.panel h3{font-size:15px;margin:0 0 8px}.muted{color:var(--muted);font-size:13px}.stats{display:grid;grid-template-columns:repeat(3,1fr);gap:8px}.stat{background:var(--panel2);border-radius:9px;padding:10px}.stat span{display:block;font-size:12px;color:var(--muted)}.stat strong{font-size:21px;font-variant-numeric:tabular-nums}.attr{padding:11px 0;border-bottom:1px solid var(--line)}.attr:last-child{border:0}.attr-head{display:flex;justify-content:space-between;gap:10px;align-items:baseline}.attr-name{font-weight:650}.attr-value{font-variant-numeric:tabular-nums;color:var(--accent)}.attr-description{font-size:12px;color:var(--muted);margin:3px 0 8px}.attr input{margin-top:7px}.slider-meta{display:flex;justify-content:space-between;gap:8px;margin-top:7px;font-size:12px;color:var(--muted)}.slider-meta strong{color:var(--accent);font-variant-numeric:tabular-nums}.turn-controls{display:flex;align-items:center;justify-content:flex-end;gap:10px 16px;flex-wrap:wrap;width:100%;padding-top:10px;border-top:1px solid var(--line)}.turn-metric{font-size:12px;color:var(--muted);white-space:nowrap}.turn-metric strong{font-size:15px;color:var(--text);font-variant-numeric:tabular-nums}.turn-metric .negative{color:var(--bad)}.turn-controls button{white-space:nowrap}.card-list{display:grid;gap:9px}.item{background:var(--panel2);border:1px solid var(--line);border-radius:10px;padding:12px}.item-top{display:flex;gap:10px;align-items:flex-start}.item-top label{flex:1;display:flex;gap:9px;align-items:flex-start;cursor:pointer}.item-title{font-weight:650}.item-meta{font-size:12px;color:var(--muted);margin-top:2px}.item-description{font-size:13px;color:#c5d0d8;margin:8px 0 0;white-space:pre-wrap}.chip{display:inline-block;padding:2px 8px;border-radius:20px;background:#304654;color:#c7d7e1;font-size:11px}.chip.good{background:#244b3a;color:#a8e0bd}.chip.bad{background:#4b302f;color:#f0b1a8}.chip.gold{background:#4a422e;color:#eedb9e}.secret{margin-top:10px;padding:11px;border-radius:8px;border:1px solid #786a3b;background:#383321;color:#f3e4ae;white-space:pre-wrap}.event{border-left:3px solid var(--accent)}.event.negative{border-left-color:var(--bad)}.event.positive{border-left-color:var(--good)}.event img{display:block;max-width:100%;max-height:260px;object-fit:contain;border-radius:8px;margin:10px 0}.event-description{white-space:pre-wrap;color:#d4dee5;font-size:13px;margin-top:8px}.event-effects{display:flex;flex-wrap:wrap;gap:6px;margin-top:9px}.effect{font-size:12px;border-radius:6px;background:#263945;padding:3px 7px}.prediction-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:9px}.prediction{background:var(--panel2);border:1px solid var(--line);padding:10px;border-radius:9px}.prediction-row{margin-top:7px}.prediction-label{display:flex;justify-content:space-between;gap:10px;font-size:12px}.bar{height:6px;border-radius:9px;background:#0e1921;margin-top:4px;overflow:hidden}.bar i{display:block;height:100%;background:var(--accent)}.actions{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-top:14px}.primary{background:#286b8d;border-color:#4e8fab;font-weight:650}.notice{border:1px solid #796d42;background:#342f20;color:#e9dca8;border-radius:9px;padding:10px;font-size:13px;margin:10px 0}.empty{padding:18px;border:1px dashed #486071;border-radius:9px;text-align:center;color:var(--muted);font-size:13px}.history summary{cursor:pointer;color:var(--accent);font-size:13px}.hint-locked{opacity:.65}.small{font-size:12px;color:var(--muted)}.error-box{background:#472d2d;border:1px solid #82514e;color:#ffd0ca;padding:10px;border-radius:8px;margin:12px 0;white-space:pre-wrap}
@media(max-width:900px){.grid{grid-template-columns:1fr}header{position:sticky;top:0}.status{margin-left:0;width:100%}.turn-controls{justify-content:flex-start}}@media(max-width:520px){main{padding:0 10px 40px}.stats{grid-template-columns:repeat(3,minmax(0,1fr))}.stat strong{font-size:17px}.panel{padding:12px}}
</style>
</head>
<body>
<header>
  <h1>Симуляция государства</h1>
  <div class="toolbar">
    <select id="scenario" aria-label="Выберите сценарий"><option value="">Загрузка сценариев…</option></select>
    <button id="start" class="primary">Начать игру</button>
    <button id="restart" class="secondary" disabled>Новая игра</button>
  </div>
  <div id="turn-controls" class="turn-controls hidden">
    <span class="turn-metric">Расход денег: <strong id="estimated-money">5</strong></span>
    <span class="turn-metric">Остаток денег: <strong id="remaining-money">—</strong></span>
    <span class="turn-metric">Расход QP: <strong id="estimated-qp">0</strong></span>
    <span class="turn-metric">Остаток QP: <strong id="remaining-qp">—</strong></span>
    <button id="make-turn" class="primary">Завершить ход</button>
  </div>
  <span id="status" class="status">Выберите сценарий</span>
</header>
<main>
  <p class="intro">Управляйте бюджетом, выбирайте стратегию и открывайте подсказки. После завершения хода события выбираются автоматически по правилам сценария.</p>
  <div id="error" class="error-box hidden"></div>
  <section id="welcome" class="panel">
    <h2>Начало игры</h2>
    <p class="muted">Выберите сценарий в верхней панели. Каждая партия создаётся отдельно и хранится на сервере до 12 часов бездействия.</p>
  </section>
  <section id="game" class="hidden">
    <div class="grid">
      <div class="stack">
        <section class="panel">
          <h2 id="game-title">Состояние государства</h2>
          <div class="stats">
            <div class="stat"><span>Деньги</span><strong id="money">0</strong><span id="max-money">из 0</span></div>
            <div class="stat"><span>Очки вопросов</span><strong id="qp">0</strong></div>
            <div class="stat"><span>Текущий ход</span><strong id="iteration">1</strong></div>
          </div>
        </section>
        <section class="panel">
          <h2>Распределение бюджета</h2>
          <p class="muted">Значения настраиваемых атрибутов задаются полностью, а не прибавляются к предыдущим. Стоимость зависит от CostPerUnit.</p>
          <div id="attributes"></div>
        </section>
        <section class="panel">
          <h2>Стратегии</h2>
          <p class="muted">Выбранный набор заменяет действующий набор стратегий на этот ход. Стоимость выбранных стратегий списывается каждый ход.</p>
          <div id="strategies" class="card-list"></div>
        </section>
        <section class="panel">
          <h2>Подсказки</h2>
          <p class="muted">Доступные подсказки можно использовать за QP. Секретная часть показывается после использования.</p>
          <div id="hints" class="card-list"></div>
        </section>
      </div>
      <div class="stack">
        <section class="panel">
          <h2>Результаты последнего хода</h2>
          <div id="last-turn"><div class="empty">Здесь появятся прогнозы и произошедшие события после первого хода.</div></div>
        </section>
        <section class="panel history">
          <details>
            <summary>История событий сценария</summary>
            <div id="event-history" class="card-list" style="margin-top:10px"></div>
          </details>
        </section>
      </div>
    </div>
  </section>
</main>
<script>
'use strict';
const $ = id => document.getElementById(id);
let snapshot = null;
let busy = false;
const escapeHTML = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const fmt = value => Number(value || 0).toLocaleString('ru-RU',{maximumFractionDigits:2});
const goRound = value => Math.sign(value) * Math.floor(Math.abs(value) + 0.5);
function setStatus(message, kind=''){ $('status').textContent=message; $('status').className='status '+kind; }
function showError(message){ const box=$('error'); box.textContent=message; box.classList.toggle('hidden',!message); }
async function api(url, options={}){
  const response=await fetch(url,{...options,headers:{'Content-Type':'application/json',...(options.headers||{})}});
  const text=await response.text();
  let data={};
  try{data=text?JSON.parse(text):{};}catch(_){throw new Error(text||('HTTP '+response.status));}
  if(!response.ok) throw new Error(data.error||text||('HTTP '+response.status));
  return data;
}
async function loadScenarios(){
  try{
    const scenarios=await api('/api/play/scenarios');
    $('scenario').innerHTML=scenarios.length?scenarios.map(item=>'<option value="'+escapeHTML(item.id)+'">'+escapeHTML(item.title)+'</option>').join(''):'<option value="">Нет доступных сценариев</option>';
    $('start').disabled=!scenarios.length;
    setStatus(scenarios.length?'Выберите сценарий':'Сценарии не найдены');
  }catch(error){showError(error.message);setStatus('Ошибка загрузки сценариев','error');$('start').disabled=true;}
}
async function startGame(){
  if(busy)return;
  const scenario=$('scenario').value;
  if(!scenario){showError('Сначала выберите сценарий.');return;}
  setBusy(true);showError('');
  try{
    snapshot=await api('/api/play/start',{method:'POST',body:JSON.stringify({scenario})});
    sessionStorage.setItem('gameplaySessionId',snapshot.sessionId);
    render();
    setStatus('Игра начата','good');
  }catch(error){showError(error.message);setStatus('Не удалось начать игру','error');}
  finally{setBusy(false);}
}
function render(){
  if(!snapshot)return;
  $('welcome').classList.add('hidden');$('game').classList.remove('hidden');$('turn-controls').classList.remove('hidden');$('restart').disabled=false;
  $('game-title').textContent='Государство: '+snapshot.scenario;
  $('money').textContent=fmt(snapshot.money);$('max-money').textContent='из '+fmt(snapshot.maxMoney);
  $('qp').textContent=fmt(snapshot.qp);$('iteration').textContent=fmt(snapshot.iteration);
  renderAttributes();renderStrategies();renderHints();renderLastTurn();renderHistory();updateEstimate();
}
function renderAttributes(){
  $('attributes').innerHTML=snapshot.attributes.map(attribute=>{
    const title='<div class="attr-head"><span class="attr-name">'+escapeHTML(attribute.name)+'</span><span class="attr-value">Сейчас: '+fmt(attribute.finalValue)+'</span></div>';
    const desc=attribute.description?'<div class="attr-description">'+escapeHTML(attribute.description)+'</div>':'';
    if(!attribute.configurable)return '<div class="attr">'+title+desc+'</div>';
    return '<div class="attr">'+title+desc+
      '<div class="slider-meta"><span>Выбрано: <strong id="config-output-'+attribute.id+'">'+fmt(attribute.baseValue)+'</strong></span><span>'+fmt(attribute.min)+'–'+fmt(attribute.max)+' · '+fmt(attribute.costPerUnit)+' за единицу</span></div>'+
      '<input class="config-value" type="range" step="any" min="'+attribute.min+'" max="'+attribute.max+'" data-id="'+attribute.id+'" value="'+attribute.baseValue+'" aria-label="'+escapeHTML(attribute.name)+'"></div>';
  }).join('')||'<div class="empty">В сценарии нет атрибутов.</div>';
  document.querySelectorAll('.config-value').forEach(input=>input.addEventListener('input',updateEstimate));
}
function prerequisiteText(req){
  const parts=[];
  if(req.events?.length)parts.push('любое событие: '+req.events.join(', '));
  if(req.strategies?.length)parts.push('любая стратегия: '+req.strategies.join(', '));
  return parts.length?'Условия: '+parts.join('; '):'Без предварительных условий';
}
function renderStrategies(){
  $('strategies').innerHTML=snapshot.strategies.map(strategy=>{
    const checked=strategy.active?'checked':'';
    return '<article class="item"><div class="item-top"><label><input class="strategy-choice" type="checkbox" value="'+strategy.id+'" '+checked+'><span><span class="item-title">'+escapeHTML(strategy.name||('Стратегия '+strategy.id))+'</span><span class="item-meta">ID '+strategy.id+' · стоимость '+strategy.cost+'</span></span></label><span class="chip '+(strategy.active?'good':'')+'">'+(strategy.active?'Активна':'Не активна')+'</span></div>'+(strategy.description?'<p class="item-description">'+escapeHTML(strategy.description)+'</p>':'')+'<p class="item-meta">'+escapeHTML(prerequisiteText(strategy.prerequisites))+'</p></article>';
  }).join('')||'<div class="empty">Пока нет доступных стратегий: выполните необходимые условия сценария.</div>';
  document.querySelectorAll('.strategy-choice').forEach(input=>input.addEventListener('change',updateEstimate));
}
function renderHints(){
  $('hints').innerHTML=snapshot.hints.map(hint=>{
    const status=hint.used?'<span class="chip good">Использована</span>':hint.available?'<span class="chip gold">Доступна</span>':'<span class="chip">Закрыта</span>';
    const checkbox=hint.available?'<input class="hint-choice" type="checkbox" value="'+hint.id+'">':'';
    const desc=hint.description?'<p class="item-description">'+escapeHTML(hint.description)+'</p>':'';
    const secret=hint.used&&hint.secret?'<div class="secret"><strong>Секрет подсказки</strong><br>'+escapeHTML(hint.secret)+'</div>':'';
    const reliability=hint.reliability?'<span class="chip">'+escapeHTML(hint.reliability)+'</span>':'';
    return '<article class="item '+(!hint.available&&!hint.used?'hint-locked':'')+'"><div class="item-top"><label>'+checkbox+'<span><span class="item-title">'+escapeHTML(hint.name||('Подсказка '+hint.id))+'</span><span class="item-meta">ID '+hint.id+' · '+hint.cost+' QP</span></span></label>'+status+'</div><div class="actions" style="margin-top:6px">'+reliability+'</div>'+desc+(hint.used?'': '<p class="item-meta">'+escapeHTML(prerequisiteText(hint.prerequisites))+'</p>')+secret+'</article>';
  }).join('')||'<div class="empty">Пока нет доступных подсказок.</div>';
  document.querySelectorAll('.hint-choice').forEach(input=>input.addEventListener('change',updateEstimate));
}
function renderLastTurn(){
  const turn=snapshot.lastTurn;
  if(!turn){$('last-turn').innerHTML='<div class="empty">Здесь появятся прогнозы и произошедшие события после первого хода.</div>';return;}
  let html='<p class="muted">Ход '+turn.turn+' · прогноз рассчитан после распределения бюджета и активации стратегий.</p>';
  if(turn.predictions?.length){
    html+='<h3>Прогнозы по веткам</h3><div class="prediction-grid">'+turn.predictions.map(branch=>{
      let body='';
      if(!branch.eligible)body='<p class="small">Сумма весов '+fmt(branch.sum)+'. Порог '+fmt(snapshot.minProbabilitySum)+' не достигнут.</p>';
      else body=branch.events.map(event=>'<div class="prediction-row"><div class="prediction-label"><span>'+escapeHTML(event.name)+'</span><strong>'+fmt(event.share)+'%</strong></div><div class="bar"><i style="width:'+Math.max(0,Math.min(100,event.share))+'%"></i></div></div>').join('');
      return '<div class="prediction"><strong>'+escapeHTML(branch.branch)+'</strong><div class="small">Сумма исходных весов: '+fmt(branch.sum)+'</div>'+body+'</div>';
    }).join('')+'</div>';
  }else html+='<p class="muted">Доступных событий с положительным весом не было.</p>';
  html+='<h3 style="margin-top:16px">Сработавшие события</h3>';
  if(turn.events?.length)html+='<div class="card-list">'+turn.events.map(eventCard).join('')+'</div>';
  else html+='<div class="empty">В этом ходу ни одно событие не сработало.</div>';
  $('last-turn').innerHTML=html;
}
function eventCard(event){
  const effects=[];
  for(const change of event.attributeChanges||[])effects.push(escapeHTML(change.name)+': '+(change.shift>0?'+':'')+fmt(change.shift));
  if(event.moneyShift)effects.push('Деньги: '+(event.moneyShift>0?'+':'')+event.moneyShift);
  if(event.qpShift)effects.push('QP: '+(event.qpShift>0?'+':'')+event.qpShift);
  const img=event.imageURL?'<img loading="lazy" src="'+escapeHTML(resolveImage(event.imageURL))+'" alt="Иллюстрация события">':'';
  return '<article class="item event '+(event.positive?'positive':'negative')+'"><div class="item-top"><div style="flex:1"><div class="item-title">'+event.id+'. '+escapeHTML(event.name)+'</div><div class="item-meta">'+escapeHTML(event.branch)+' · итерация '+event.iteration+'</div></div><span class="chip '+(event.positive?'good':'bad')+'">'+(event.positive?'Положительное':'Отрицательное')+'</span></div>'+img+'<div class="event-description">'+escapeHTML(event.description)+'</div>'+(effects.length?'<div class="event-effects">'+effects.map(effect=>'<span class="effect">'+effect+'</span>').join('')+'</div>':'')+'</article>';
}
function resolveImage(path){
  if(/^https?:\/\//i.test(path))return path;
  return '/api/image?name='+encodeURIComponent(snapshot.scenario)+'&file='+encodeURIComponent(path);
}
function renderHistory(){
  const events=(snapshot.events||[]).filter(event=>event.status!=='neutral');
  $('event-history').innerHTML=events.length?events.map(event=>'<div class="item"><div class="item-title">'+event.id+'. '+escapeHTML(event.name)+'</div><div class="item-meta">'+escapeHTML(event.branch)+' · '+(event.status==='fired'?'Сработало в последнем ходу':'Произошло ранее')+'</div></div>').join(''):'<div class="empty">Событий пока не было.</div>';
}
function updateEstimate(){
  if(!snapshot)return;
  let money=5,qp=0;
  document.querySelectorAll('.config-value').forEach(input=>{
    const id=Number(input.dataset.id),attribute=snapshot.attributes.find(item=>item.id===id);
    const value=Number(input.value);
    const output=$('config-output-'+id);
    if(output)output.textContent=fmt(value);
    if(attribute&&Number.isFinite(value))money+=goRound(value*attribute.costPerUnit);
  });
  document.querySelectorAll('.strategy-choice:checked').forEach(input=>{
    const strategy=snapshot.strategies.find(item=>item.id===Number(input.value));if(strategy)money+=strategy.cost;
  });
  document.querySelectorAll('.hint-choice:checked').forEach(input=>{
    const hint=snapshot.hints.find(item=>item.id===Number(input.value));if(hint)qp+=hint.cost;
  });
  const moneyLeft=snapshot.money-money;
  const qpLeft=snapshot.qp-qp;
  $('estimated-money').textContent=fmt(money);
  $('estimated-qp').textContent=fmt(qp);
  $('remaining-money').textContent=fmt(moneyLeft);
  $('remaining-qp').textContent=fmt(qpLeft);
  $('remaining-money').classList.toggle('negative',moneyLeft<0);
  $('remaining-qp').classList.toggle('negative',qpLeft<0);
  $('make-turn').disabled=busy||moneyLeft<0||qpLeft<0;
}
async function makeTurn(){
  if(!snapshot||busy)return;
  const attributes=[...document.querySelectorAll('.config-value')].map(input=>({id:Number(input.dataset.id),value:Number(input.value)}));
  if(attributes.some(item=>!Number.isFinite(item.value))){showError('Проверьте значения настраиваемых атрибутов.');return;}
  const strategies=[...document.querySelectorAll('.strategy-choice:checked')].map(input=>Number(input.value));
  const hints=[...document.querySelectorAll('.hint-choice:checked')].map(input=>Number(input.value));
  setBusy(true);showError('');
  try{
    snapshot=await api('/api/play/turn',{method:'POST',body:JSON.stringify({sessionId:snapshot.sessionId,attributes,strategies,hints})});
    render();setStatus('Ход завершён','good');
  }catch(error){showError(error.message);setStatus('Ход не выполнен','error');}
  finally{setBusy(false);updateEstimate();}
}
function setBusy(value){busy=value;$('start').disabled=value;$('make-turn').disabled=value;$('restart').disabled=value||!snapshot;if(!value&&snapshot)updateEstimate();}
$('start').addEventListener('click',startGame);
$('restart').addEventListener('click',startGame);
$('make-turn').addEventListener('click',makeTurn);
async function restoreGame(){
  const sessionId=sessionStorage.getItem('gameplaySessionId');
  if(!sessionId)return;
  try{
    snapshot=await api('/api/play/state?sessionId='+encodeURIComponent(sessionId));
    render();setStatus('Продолжена сохранённая сессия','good');
  }catch(error){
    sessionStorage.removeItem('gameplaySessionId');
    setStatus('Сохранённая сессия недоступна');
  }
}
loadScenarios().then(restoreGame);
</script>
</body>
</html>`
