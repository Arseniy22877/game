package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/Arseniy22877/game/internal/enginebridge"
	"github.com/Arseniy22877/game/internal/game"
	"github.com/Arseniy22877/game/internal/store"
	"github.com/Arseniy22877/game/internal/ws"
)

func TestRoundRequestValidationAndZeroValues(t *testing.T) {
	const (
		adminToken   = "instructor-token"
		decisionPath = "/api/team/decision"
		closePath    = "/api/admin/round/close"
		eventsPath   = "/api/admin/round/events"
		finishPath   = "/api/admin/game/finish"
	)
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	scenarios := t.TempDir()
	writeFixture(t, filepath.Join(scenarios, "zero.json"), `{
		"ConfigurableAttributes": [{"ID":0,"Name":"Slider","MinValue":0,"MaxValue":1,"DefaultValue":1,"CostPerUnit":2}],
		"Branches": ["main"],
		"Events": [
			{"ID":1,"Name":"Opening","Branch":"main","BasicPower":1,"Weights":[0]},
			{"ID":0,"Name":"First event","Branch":"main","BasicPower":1,"Weights":[0]}
		],
		"OpeningEvent": 1,
		"StartingBudget": {"InitialMoney":20,"MaxMoney":20},
		"Strategies": [{"ID":0,"Name":"Strategy","Cost":0}],
		"Hints": [{"ID":0,"Name":"Hint","Cost":0,"Secret":"Research"}]
	}`)
	catalog, err := enginebridge.LoadCatalog(scenarios)
	if err != nil {
		t.Fatal(err)
	}
	hub := ws.New(nil)
	t.Cleanup(hub.Close)
	manager, err := game.NewManager(ctx, db, catalog, hub)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(slog.New(slog.DiscardHandler), t.TempDir(), scenarios, nil, manager, adminToken)
	post := func(path, token, body string) *httptest.ResponseRecorder {
		return requestAPI(handler, http.MethodPost, path, token, body)
	}

	var state api.AdminState
	decodeAPI(t, post("/api/admin/game", adminToken, ""), http.StatusCreated, &state)
	gameID := state.Game.GameID
	var team api.RegisterTeamResponse
	decodeAPI(t, post("/api/teams", "", `{"name":"Команда","members":["Аня"]}`), http.StatusCreated, &team)
	decodeAPI(t, post("/api/admin/game/start", adminToken, `{"scenarioId":"zero"}`), http.StatusOK, &state)
	assertError(t, post(finishPath, adminToken, "{"), http.StatusBadRequest, api.CodeBadRequest)
	const validDecision = `{"round":1,"attributes":[{"id":0,"value":0}],"strategies":[0],"hints":[0],"report":{"why":"","alternative":"","risks":"","correction":""}}`
	for _, token := range []string{"", adminToken, "unknown-team"} {
		assertError(t, post(decisionPath, token, validDecision), http.StatusUnauthorized, api.CodeUnauthorized)
	}
	for _, path := range []string{closePath, eventsPath, finishPath} {
		assertError(t, post(path, "", `{}`), http.StatusUnauthorized, api.CodeUnauthorized)
		assertError(t, post(path, team.Token, `{}`), http.StatusUnauthorized, api.CodeUnauthorized)
	}
	for _, body := range []string{
		strings.Replace(validDecision, `"round":1,`, "", 1),
		strings.Replace(validDecision, `"id":0,`, "", 1),
		strings.Replace(validDecision, `,"value":0`, "", 1),
		strings.Replace(validDecision, `"value":0`, `"value":null`, 1),
		strings.Replace(validDecision, `"hints":[0]`, `"hints":[null]`, 1),
		strings.Replace(validDecision, `"strategies":[0]`, `"strategies":[null]`, 1),
		strings.Replace(validDecision, `,"correction":""`, "", 1),
	} {
		assertError(t, post(decisionPath, team.Token, body), http.StatusBadRequest, api.CodeBadRequest)
	}
	assertError(t, post(closePath, adminToken, `{}`), http.StatusBadRequest, api.CodeBadRequest)
	assertError(t, post(closePath, adminToken, `{"round":null}`), http.StatusBadRequest, api.CodeBadRequest)
	assertError(t, post(closePath, adminToken, `{"round":1,"force":false}`), http.StatusConflict, api.CodeNotAllSubmitted)
	var submitted api.SubmitDecisionResponse
	decodeAPI(t, post(decisionPath, team.Token, validDecision), http.StatusOK, &submitted)
	if submitted.Cost != (api.Cost{Money: state.Game.BaseRoundCost, QP: 0}) {
		t.Fatalf("cost at zero = %+v", submitted.Cost)
	}
	decodeAPI(t, post(closePath, adminToken, `{"round":1,"force":false}`), http.StatusOK, &state)
	if state.Game.Phase != api.PhaseRoundClosed || len(state.Teams) != 1 || !state.Teams[0].Submitted {
		t.Fatalf("close response = %+v", state)
	}
	decision := api.Decision{Attributes: []api.AttributeChoice{{ID: 0, Value: 0}}, Strategies: []int{0}, Hints: []int{0}}
	if !reflect.DeepEqual(state.Teams[0].Decision, &decision) {
		t.Fatalf("decision = %+v, want %+v", state.Teams[0].Decision, decision)
	}
	for _, body := range []string{
		`{"selections":[]}`,
		`{"round":1}`,
		`{"round":1,"selections":null}`,
		`{"round":1,"selections":[{"eventIds":[0]}]}`,
		fmt.Sprintf(`{"round":1,"selections":[{"teamId":%d}]}`, team.Team.ID),
		fmt.Sprintf(`{"round":1,"selections":[{"teamId":%d,"eventIds":null}]}`, team.Team.ID),
		fmt.Sprintf(`{"round":1,"selections":[{"teamId":%d,"eventIds":[null]}]}`, team.Team.ID),
	} {
		assertError(t, post(eventsPath, adminToken, body), http.StatusBadRequest, api.CodeBadRequest)
	}
	assertError(t, post(eventsPath, adminToken, `{"round":1,"selections":[]}`), http.StatusConflict, api.CodeNotAllTeams)
	invalidEvent := fmt.Sprintf(`{"round":1,"selections":[{"teamId":%d,"eventIds":[0,0]}]}`, team.Team.ID)
	assertError(t, post(eventsPath, adminToken, invalidEvent), http.StatusUnprocessableEntity, api.CodeInvalidEvent)
	validEvent := fmt.Sprintf(`{"round":1,"finish":true,"selections":[{"teamId":%d,"eventIds":[0]}]}`, team.Team.ID)
	decodeAPI(t, post(eventsPath, adminToken, validEvent), http.StatusOK, &state)
	if state.Game.Phase != api.PhaseFinished || state.Game.Round != 1 || len(state.Teams) != 1 {
		t.Fatalf("finish response = %+v", state)
	}
	if history := state.Teams[0].State.History; len(history) != 1 || len(history[0].Events) != 1 || history[0].Events[0].ID != 0 {
		t.Fatalf("event ID 0 did not round-trip: %+v", history)
	}
	var detail api.GameDetail
	decodeAPI(t, requestAPI(handler, http.MethodGet, fmt.Sprintf("/api/admin/games/%d", gameID), adminToken, ""), http.StatusOK, &detail)
	if len(detail.Teams) != 1 || len(detail.Teams[0].Rounds) != 1 {
		t.Fatalf("game detail = %+v", detail)
	}
	round := detail.Teams[0].Rounds[0]
	if !reflect.DeepEqual(round.Decision, &decision) || len(round.Events) != 1 || round.Events[0].ID != 0 {
		t.Fatalf("zero IDs or values were lost in SQLite: %+v", round)
	}
}
