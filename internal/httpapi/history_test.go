package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/Arseniy22877/game/internal/enginebridge"
	"github.com/Arseniy22877/game/internal/game"
	"github.com/Arseniy22877/game/internal/store"
)

func TestHistoryAndGrades(t *testing.T) {
	const adminToken = "instructor-token"
	const gamesPath = "/api/admin/games"
	ctx := t.Context()
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "game.db"))
	check(err)
	t.Cleanup(func() { _ = db.Close() })
	scenarios := t.TempDir()
	scenarioPath := filepath.Join(scenarios, "full.json")
	raw, err := os.ReadFile(filepath.Join("..", "enginebridge", "testdata", "full.json"))
	check(err)
	check(os.WriteFile(scenarioPath, raw, 0o600))
	catalog, err := enginebridge.LoadCatalog(scenarios)
	check(err)
	manager, err := game.NewManager(ctx, db, catalog, nil)
	check(err)
	handler := NewServer(slog.New(slog.DiscardHandler), t.TempDir(), scenarios, nil, manager, adminToken)
	request := func(method, path, token, body string) *httptest.ResponseRecorder {
		return requestAPI(handler, method, path, token, body)
	}
	decode := func(response *httptest.ResponseRecorder, target any) {
		t.Helper()
		decodeAPI(t, response, http.StatusOK, target)
	}
	var games api.GamesResponse
	decode(request(http.MethodGet, gamesPath, adminToken, ""), &games)
	if games.Games == nil || len(games.Games) != 0 {
		t.Fatalf("empty history = %#v", games.Games)
	}

	// Seed results through the SQLite journal to control the snapshots and predictions.
	finished, err := db.CreateGame(ctx)
	check(err)
	first, err := db.RegisterTeam(ctx, finished.ID, "Первая", []string{"Аня"}, "team-token")
	check(err)
	second, err := db.RegisterTeam(ctx, finished.ID, "Вторая", []string{"Петя"}, "other-team-token")
	check(err)
	gamePath := fmt.Sprintf("%s/%d", gamesPath, finished.ID)
	gradePath := fmt.Sprintf("%s/teams/%d/grades", gamePath, first.ID)
	getDetail := func(path string) api.GameDetail {
		t.Helper()
		var detail api.GameDetail
		decode(request(http.MethodGet, path, adminToken, ""), &detail)
		return detail
	}
	for _, token := range []string{"", first.Token} {
		assertError(t, request(http.MethodGet, gamesPath, token, ""), http.StatusUnauthorized, api.CodeUnauthorized)
		assertError(t, request(http.MethodGet, gamePath, token, ""), http.StatusUnauthorized, api.CodeUnauthorized)
		assertError(t, request(http.MethodPut, gradePath, token, `{"grades":[]}`), http.StatusUnauthorized, api.CodeUnauthorized)
	}
	assertError(t, request(http.MethodGet, gamesPath+"/9999", adminToken, ""), http.StatusNotFound, api.CodeNotFound)
	assertError(t, request(http.MethodPut, gradePath, adminToken, `{"grades":[]}`), http.StatusConflict, api.CodeWrongPhase)

	check(db.StartGame(ctx, finished.ID, "full", raw))
	credit := api.Event{ID: 0, Name: "Кредит"}
	penalty := api.Event{ID: 7, Name: "Штраф"}
	predictions := []api.Prediction{{Event: credit, Suggested: true}, {Event: penalty}}
	check(db.CloseRound(ctx, finished.ID, 1, []store.RoundCalculation{
		{TeamID: first.ID, AutoDecision: &store.DecisionRecord{Cost: api.Cost{Money: 5}}, State: api.TeamGameState{Budget: api.Budget{Money: 8}}},
		{TeamID: second.ID, Predictions: predictions},
	}))
	pending := getDetail(gamePath)
	if pending.Game.Rounds != 0 || pending.Game.Phase != api.PhaseRoundClosed || !pending.Game.Active || len(pending.Teams) != 2 {
		t.Fatalf("pending detail = %+v", pending)
	}
	for _, team := range pending.Teams {
		if len(team.Rounds) != 1 || team.Grades == nil {
			t.Fatalf("pending team = %+v", team)
		}
	}
	firstRound := pending.Teams[0].Rounds[0]
	if !firstRound.Auto || firstRound.Cost == nil || firstRound.Cost.Money != 5 || firstRound.Decision == nil ||
		firstRound.Events == nil || len(firstRound.Events) != 0 || firstRound.State.Budget.Money != 8 ||
		firstRound.State.History == nil || len(firstRound.State.History) != 0 {
		t.Fatalf("pending round = %+v", firstRound)
	}
	if skipped := pending.Teams[1].Rounds[0]; skipped.Decision != nil || skipped.Cost != nil || skipped.Auto {
		t.Fatalf("skipped round = %+v", skipped)
	}
	check(db.ApplyEvents(ctx, finished.ID, 1, []store.RoundEventSelection{
		{TeamID: first.ID, EventIDs: []int{}, State: api.TeamGameState{Budget: api.Budget{Money: 8}}},
		{TeamID: second.ID, EventIDs: []int{penalty.ID, credit.ID}},
	}, true))
	complete := getDetail(gamePath)
	if complete.Game.Phase != api.PhaseFinished || complete.Game.Rounds != 1 || !complete.Game.Active {
		t.Fatalf("finished game = %+v", complete.Game)
	}
	played := complete.Teams[0].Rounds[0]
	if !reflect.DeepEqual(played.State.History, []api.RoundEvents{{Round: 1, Events: []api.Event{}}}) {
		t.Fatalf("confirmed empty selection = %+v", played)
	}
	if secondRound := complete.Teams[1].Rounds[0]; !reflect.DeepEqual(secondRound.Events, []api.Event{penalty, credit}) ||
		!reflect.DeepEqual(secondRound.Predictions, predictions) {
		t.Fatalf("history changed saved event order or predictions: %+v", secondRound)
	}
	check(os.WriteFile(scenarioPath, []byte(`{"updated":true}`), 0o600))
	if detail := getDetail(gamePath); !reflect.DeepEqual(detail, complete) {
		t.Fatalf("history changed after editing the scenario file: %+v", detail)
	}

	var result api.Grades
	decode(request(http.MethodPut, gradePath, adminToken, `{"grades":[{"skill":"risks","score":0,"comment":"  Проверено  "}]}`), &result)
	previous := []api.Grade{{Skill: api.SkillRisks, Comment: "Проверено"}}
	if !reflect.DeepEqual(result.Grades, previous) {
		t.Fatalf("saved grades = %+v", result.Grades)
	}
	for _, body := range []string{
		`{}`, `{"grades":null}`, `{"grades":[]} {}`,
		`{"grades":[{"skill":"risks"}]}`, `{"grades":[{"skill":"risks","score":null}]}`,
		`{"grades":[{"skill":"risks","score":0.5}]}`, `{"grades":[{"skill":"risks","score":-1}]}`,
		`{"grades":[{"skill":"risks","score":11}]}`, `{"grades":[{"skill":"unknown","score":1}]}`,
		`{"grades":[{"skill":"risks","score":1},{"skill":"risks","score":2}]}`,
		fmt.Sprintf(`{"grades":[{"skill":"risks","score":8,"comment":%q}]}`, strings.Repeat("я", 2001)),
	} {
		assertError(t, request(http.MethodPut, gradePath, adminToken, body), http.StatusBadRequest, api.CodeBadRequest)
	}
	if grades := getDetail(gamePath).Teams[0].Grades; !reflect.DeepEqual(grades, previous) {
		t.Fatalf("invalid grades changed the previous result: %+v", grades)
	}

	interrupted, err := db.CreateGame(ctx)
	check(err)
	third, err := db.RegisterTeam(ctx, interrupted.ID, "Третья", []string{"Маша"}, "third-team-token")
	check(err)
	thirdPath := fmt.Sprintf("%s/%d", gamesPath, interrupted.ID)
	assertError(t, request(http.MethodPut, fmt.Sprintf("%s/teams/%d/grades", thirdPath, first.ID), adminToken, `{"grades":[]}`), http.StatusNotFound, api.CodeNotFound)
	check(db.StartGame(ctx, interrupted.ID, "full", raw))
	check(db.CloseRound(ctx, interrupted.ID, 1, []store.RoundCalculation{{TeamID: third.ID}}))
	current, err := db.CreateGame(ctx)
	check(err)
	decode(request(http.MethodGet, gamesPath, adminToken, ""), &games)
	if len(games.Games) != 3 || games.Games[0].GameID != current.ID || !games.Games[0].Active ||
		games.Games[1].GameID != interrupted.ID || games.Games[1].Active || games.Games[1].Phase != api.PhaseRoundClosed || games.Games[1].Rounds != 0 ||
		games.Games[2].GameID != finished.ID || games.Games[2].Active || games.Games[2].Phase != api.PhaseFinished || games.Games[2].Rounds != 1 {
		t.Fatalf("game history = %+v", games.Games)
	}
	comment := strings.Repeat("я", 2000)
	decode(request(http.MethodPut, gradePath, adminToken, fmt.Sprintf(`{"grades":[{"skill":"revision","score":10,"comment":%q}]}`, comment)), &result)
	if grades := getDetail(gamePath).Teams[0].Grades; !reflect.DeepEqual(grades, []api.Grade{{Skill: api.SkillRevision, Score: 10, Comment: comment}}) {
		t.Fatalf("archived game grades were not replaced: %+v", grades)
	}
	decode(request(http.MethodPut, fmt.Sprintf("%s/teams/%d/grades", thirdPath, third.ID), adminToken, `{"grades":[{"skill":"risks","score":8}]}`), &result)
	if detail := getDetail(thirdPath); detail.Game.Active || len(detail.Teams[0].Grades) != 1 {
		t.Fatalf("interrupted game's grades = %+v", detail)
	}
	decode(request(http.MethodPut, gradePath, adminToken, `{"grades":[]}`), &result)
	if grades := getDetail(gamePath).Teams[0].Grades; result.Grades == nil || len(result.Grades) != 0 || grades == nil || len(grades) != 0 {
		t.Fatalf("grades not cleared: response %v, stored %v", result.Grades, grades)
	}
}

func requestAPI(handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, r)
	return response
}

func decodeAPI(t *testing.T, response *httptest.ResponseRecorder, status int, target any) {
	t.Helper()
	if response.Code != status || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("response %d, headers %v: %s", response.Code, response.Header(), response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.String(), err)
	}
}
