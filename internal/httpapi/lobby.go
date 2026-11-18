package httpapi

import (
	"net/http"
	"strings"

	"github.com/Arseniy22877/game/internal/api"
)

func (s *serverAPI) lobbyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/game", s.gameInfo)
	mux.HandleFunc("POST /api/teams", s.registerTeam)
	mux.HandleFunc("PUT /api/team/ready", s.team(s.setReady))
	mux.HandleFunc("GET /api/team/state", s.team(s.teamState))
	mux.HandleFunc("GET /api/admin/scenarios", s.admin(s.scenarios))
	mux.HandleFunc("POST /api/admin/game", s.admin(s.createGame))
	mux.HandleFunc("POST /api/admin/game/start", s.admin(s.startGame))
	mux.HandleFunc("DELETE /api/admin/teams/{teamId}", s.admin(s.deleteTeam))
	mux.HandleFunc("GET /api/admin/state", s.admin(s.adminState))
}

func (s *serverAPI) gameInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.manager.ReadGameInfo()
	s.respond(w, info, err)
}

func (s *serverAPI) registerTeam(w http.ResponseWriter, r *http.Request) {
	var request api.RegisterTeamRequest
	if err := readJSON(r, &request, false); err != nil {
		s.respond(w, nil, err)
		return
	}
	registration, err := s.manager.RegisterTeam(r.Context(), request)
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	writeJSON(w, http.StatusCreated, registration)
}

func (s *serverAPI) setReady(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Ready *bool `json:"ready"`
	}
	if err := readJSON(r, &request, false); err != nil {
		s.respond(w, nil, err)
		return
	}
	if request.Ready == nil {
		s.respond(w, nil, badRequest("Укажите готовность команды"))
		return
	}
	team, err := s.manager.SetReady(r.Context(), teamToken(r), *request.Ready)
	s.respond(w, team, err)
}

func (s *serverAPI) teamState(w http.ResponseWriter, r *http.Request) {
	state, err := s.manager.TeamState(teamToken(r))
	s.respond(w, state, err)
}

func (s *serverAPI) scenarios(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.manager.Scenarios())
}

func (s *serverAPI) createGame(w http.ResponseWriter, r *http.Request) {
	var request api.CreateGameRequest
	if err := readJSON(r, &request, true); err != nil {
		s.respond(w, nil, err)
		return
	}
	state, err := s.manager.CreateLobby(r.Context(), request.Confirm)
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	writeJSON(w, http.StatusCreated, state)
}

func (s *serverAPI) startGame(w http.ResponseWriter, r *http.Request) {
	var request api.StartGameRequest
	if err := readJSON(r, &request, false); err != nil {
		s.respond(w, nil, err)
		return
	}
	id := strings.TrimSpace(request.ScenarioID)
	if id == "" {
		s.respond(w, nil, badRequest("Укажите сценарий игры"))
		return
	}
	state, err := s.manager.StartGame(r.Context(), id)
	s.respond(w, state, err)
}

func (s *serverAPI) deleteTeam(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "teamId")
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	state, err := s.manager.DeleteTeam(r.Context(), id)
	s.respond(w, state, err)
}

func (s *serverAPI) adminState(w http.ResponseWriter, r *http.Request) {
	state, err := s.manager.ReadAdminState()
	s.respond(w, state, err)
}
