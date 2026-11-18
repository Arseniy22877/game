package httpapi

import (
	"net/http"

	"github.com/Arseniy22877/game/internal/api"
)

func (s *serverAPI) historyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/games", s.admin(s.games))
	mux.HandleFunc("GET /api/admin/games/{gameId}", s.admin(s.gameDetail))
	mux.HandleFunc("PUT /api/admin/games/{gameId}/teams/{teamId}/grades", s.admin(s.setGrades))
}

func (s *serverAPI) games(w http.ResponseWriter, r *http.Request) {
	result, err := s.manager.Games(r.Context())
	s.respond(w, result, err)
}

func (s *serverAPI) gameDetail(w http.ResponseWriter, r *http.Request) {
	gameID, err := pathID(r, "gameId")
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	result, err := s.manager.GameDetail(r.Context(), gameID)
	s.respond(w, result, err)
}

func (s *serverAPI) setGrades(w http.ResponseWriter, r *http.Request) {
	gameID, err := pathID(r, "gameId")
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	teamID, err := pathID(r, "teamId")
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	var body struct {
		Grades []struct {
			Skill   string `json:"skill"`
			Score   *int   `json:"score"`
			Comment string `json:"comment"`
		} `json:"grades"`
	}
	if err := readJSON(r, &body, false); err != nil {
		s.respond(w, nil, err)
		return
	}
	if body.Grades == nil {
		s.respond(w, nil, badRequest("Укажите массив оценок grades"))
		return
	}
	grades := make([]api.Grade, 0, len(body.Grades))
	for _, grade := range body.Grades {
		if grade.Score == nil {
			s.respond(w, nil, badRequest("Укажите оценку score для каждого навыка"))
			return
		}
		grades = append(grades, api.Grade{Skill: grade.Skill, Score: *grade.Score, Comment: grade.Comment})
	}
	result, err := s.manager.SetGrades(r.Context(), gameID, teamID, grades)
	s.respond(w, result, err)
}
