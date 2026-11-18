package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
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

func TestLobbyHTTPAndRecovery(t *testing.T) {
	const adminToken = "admin-token"
	const v2 = `{"scenarioId":"republic_n_scenario_v2"}`
	ctx := t.Context()
	dir := filepath.Join("..", "..", "scenarios")
	dbPath := filepath.Join(t.TempDir(), "game.db")
	webDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var handler http.Handler
	var stop func()
	restart := func() {
		t.Helper()
		if stop != nil {
			stop()
		}
		db, err := store.Open(ctx, dbPath)
		if err != nil {
			t.Fatal(err)
		}
		hub := ws.New(nil)
		stop = func() {
			hub.Close()
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		}
		catalog, err := enginebridge.LoadCatalog(dir)
		if err != nil {
			t.Fatal(err)
		}
		manager, err := game.NewManager(ctx, db, catalog, hub)
		if err != nil {
			t.Fatal(err)
		}
		handler = NewServer(logger, webDir, dir, nil, manager, adminToken)
	}
	restart()
	t.Cleanup(func() { stop() })

	request := func(method, path, token, body string, status int, code string) string {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d, expected %d. %s", method, path, w.Code, status, w.Body.String())
		}
		if strings.HasPrefix(path, "/api/") && (w.Header().Get("Cache-Control") != "no-store" ||
			!strings.HasPrefix(w.Header().Get("Content-Type"), "application/json")) {
			t.Fatalf("%s %s: unexpected headers %v", method, path, w.Header())
		}
		if code != "" {
			var response api.ErrorResponse
			decodeLobbyResponse(t, w.Body.String(), &response)
			if response.Error.Code != code || response.Error.Message == "" {
				t.Fatalf("%s %s: %s, expected error %s", method, path, w.Body.String(), code)
			}
		}
		return w.Body.String()
	}
	var info api.GameInfo
	decodeLobbyResponse(t, request("GET", "/api/game", "", "", 200, ""), &info)
	if info.Phase != api.PhaseNone || info.GameID != 0 || info.Round != 0 {
		t.Fatal(info)
	}
	request("GET", "/api/admin/scenarios", "", "", 401, api.CodeUnauthorized)
	var scenarios api.ScenariosResponse
	decodeLobbyResponse(t, request("GET", "/api/admin/scenarios", adminToken, "", 200, ""), &scenarios)
	if !reflect.DeepEqual(scenarios.Scenarios, []api.Scenario{{ID: "republic_n_scenario"}, {ID: "republic_n_scenario_v2"}}) {
		t.Fatal(scenarios)
	}
	request("POST", "/api/teams", "", `{"name":"Ромашка","members":["Аня"]}`, 409, api.CodeNoGame)
	var lobby api.AdminState
	decodeLobbyResponse(t, request("POST", "/api/admin/game", adminToken, "", 201, ""), &lobby)
	if lobby.Game.Phase != api.PhaseLobby || lobby.Teams == nil || len(lobby.Teams) != 0 {
		t.Fatal(lobby)
	}
	request("POST", "/api/admin/game/start", adminToken, v2, 409, api.CodeTeamsNotReady)
	request("POST", "/api/teams", "", `{"name":"A","members":[" "]}`, 400, api.CodeBadRequest)
	request("POST", "/api/teams", "", `{"name":"A","members":["B"]} {}`, 400, api.CodeBadRequest)
	request("POST", "/api/teams", "", `{"name":"A","members":["B"],"padding":"`+strings.Repeat("x", maxBodyBytes)+`"}`, 400, api.CodeBadRequest)

	register := func(body string) api.RegisterTeamResponse {
		t.Helper()
		var v api.RegisterTeamResponse
		decodeLobbyResponse(t, request("POST", "/api/teams", "", body, 201, ""), &v)
		if !v.Team.Ready || len(v.Token) != 32 {
			t.Fatal(v)
		}
		return v
	}
	a := register(`{"name":" Ромашка ","members":[" Аня "],"extra":true}`)
	if a.Team.Name != "Ромашка" || !reflect.DeepEqual(a.Team.Members, []string{"Аня"}) {
		t.Fatal(a.Team)
	}
	request("POST", "/api/teams", "", `{"name":" рОмАшКа ","members":["Вася"]}`, 409, api.CodeNameTaken)
	b := register(`{"name":"Василёк","members":["Маша"]}`)
	request("GET", "/api/admin/state", a.Token, "", 401, api.CodeUnauthorized)
	request("GET", "/api/team/state", adminToken, "", 401, api.CodeUnauthorized)
	request("PUT", "/api/team/ready", a.Token, `{}`, 400, api.CodeBadRequest)
	var readiness api.TeamSummary
	decodeLobbyResponse(t, request("PUT", "/api/team/ready", a.Token, `{"ready":false}`, 200, ""), &readiness)
	if readiness.Ready {
		t.Fatal(readiness)
	}
	request("POST", "/api/admin/game/start", adminToken, v2, 409, api.CodeTeamsNotReady)
	stateBefore := request("GET", "/api/admin/state", adminToken, "", 200, "")
	teamBefore := request("GET", "/api/team/state", a.Token, "", 200, "")
	var team api.TeamStateResponse
	decodeLobbyResponse(t, teamBefore, &team)
	if team.Team.Ready || team.State != nil || team.Opening != nil || team.Decision != nil {
		t.Fatal(team)
	}
	restart()
	if stateBefore != request("GET", "/api/admin/state", adminToken, "", 200, "") ||
		teamBefore != request("GET", "/api/team/state", a.Token, "", 200, "") {
		t.Fatal("lobby changed after restart")
	}
	request("PUT", "/api/team/ready", a.Token, `{"ready":true}`, 200, "")
	request("POST", "/api/admin/game/start", adminToken, `{"scenarioId":"missing"}`, 404, api.CodeNotFound)
	teamPath := fmt.Sprintf("/api/admin/teams/%d", b.Team.ID)
	request("DELETE", teamPath, adminToken, "", 200, "")
	request("DELETE", teamPath, adminToken, "", 404, api.CodeNotFound)
	request("GET", "/api/team/state", b.Token, "", 401, api.CodeUnauthorized)
	c := register(`{"name":"Василёк","members":["Маша"]}`)
	if c.Token == b.Token || c.Team.ID == b.Team.ID {
		t.Fatal("registration reused identity of deleted team")
	}
	stateBefore = request("POST", "/api/admin/game/start", adminToken, v2, 200, "")
	var started api.AdminState
	decodeLobbyResponse(t, stateBefore, &started)
	if started.Game.Phase != api.PhaseRoundOpen || started.Game.Round != 1 || len(started.Teams) != 2 ||
		started.Teams[0].ID != a.Team.ID || started.Teams[1].ID != c.Team.ID {
		t.Fatal(started)
	}
	for _, v := range started.Teams {
		if v.State == nil || v.State.Attributes == nil || v.State.Configurable == nil || v.State.Strategies == nil ||
			v.State.Hints == nil || v.State.History == nil || v.Predictions == nil || v.Bankrupt || v.Decision != nil || v.Cost != nil {
			t.Fatal(v)
		}
	}
	teamBefore = request("GET", "/api/team/state", a.Token, "", 200, "")
	decodeLobbyResponse(t, teamBefore, &team)
	if team.Opening == nil || !strings.HasPrefix(team.Opening.ImageURL, "/images/republic_n_scenario_v2/") {
		t.Fatal(team.Opening)
	}
	if len(request("GET", team.Opening.ImageURL, "", "", 200, "")) == 0 {
		t.Fatal("empty opening image")
	}
	restart()
	if stateBefore != request("GET", "/api/admin/state", adminToken, "", 200, "") ||
		teamBefore != request("GET", "/api/team/state", a.Token, "", 200, "") {
		t.Fatal("first round changed after restart")
	}
	request("POST", "/api/teams", "", `{"name":"Новая","members":["Вася"]}`, 409, api.CodeWrongPhase)
	request("PUT", "/api/team/ready", a.Token, `{"ready":false}`, 409, api.CodeWrongPhase)
	request("POST", "/api/admin/game", adminToken, "", 409, api.CodeGameInProgress)
	request("POST", "/api/admin/game", adminToken, `{"confirm":true}`, 201, "")
	request("GET", "/api/team/state", a.Token, "", 401, api.CodeUnauthorized)
}

func decodeLobbyResponse(t *testing.T, body string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), target); err != nil {
		t.Fatalf("invalid JSON %s: %v", body, err)
	}
}
