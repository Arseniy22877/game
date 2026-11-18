package httpapi

import (
	"net/http"

	"github.com/Arseniy22877/game/internal/api"
)

func (s *serverAPI) roundRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/team/decision", s.team(s.submitDecision))
	mux.HandleFunc("POST /api/admin/round/close", s.admin(s.closeRound))
	mux.HandleFunc("POST /api/admin/round/events", s.admin(s.applyEvents))
	mux.HandleFunc("POST /api/admin/game/finish", s.admin(func(w http.ResponseWriter, r *http.Request) {
		var empty struct{}
		if err := readJSON(r, &empty, true); err != nil {
			s.respond(w, nil, err)
			return
		}
		v, err := s.manager.Finish(r.Context())
		s.respond(w, v, err)
	}))
}

// IDs, slider values and report fields can legitimately be zero or empty.
type decisionInput struct {
	Round      *int
	Attributes []struct {
		ID    *int
		Value *float64
	}
	Strategies []*int
	Hints      []*int
	Report     struct{ Why, Alternative, Risks, Correction *string }
}

func ids(values []*int) ([]int, error) {
	result := make([]int, len(values))
	for index, value := range values {
		if value == nil {
			return nil, badRequest("ID должен быть целым числом")
		}
		result[index] = *value
	}
	return result, nil
}

func (d decisionInput) request() (api.SubmitDecisionRequest, error) {
	var req api.SubmitDecisionRequest
	report := d.Report
	if d.Round == nil || report.Why == nil || report.Alternative == nil || report.Risks == nil || report.Correction == nil {
		return req, badRequest("Укажите раунд и все четыре поля отчёта")
	}
	req.Round = *d.Round
	req.Report = api.Report{Why: *report.Why, Alternative: *report.Alternative, Risks: *report.Risks, Correction: *report.Correction}
	req.Attributes = make([]api.AttributeChoice, len(d.Attributes))
	for i, attr := range d.Attributes {
		if attr.ID == nil || attr.Value == nil {
			return req, badRequest("Для атрибута нужны id и value")
		}
		req.Attributes[i] = api.AttributeChoice{ID: *attr.ID, Value: *attr.Value}
	}
	var err error
	if req.Strategies, err = ids(d.Strategies); err != nil {
		return req, err
	}
	req.Hints, err = ids(d.Hints)
	return req, err
}

func (s *serverAPI) submitDecision(w http.ResponseWriter, r *http.Request) {
	var input decisionInput
	if err := readJSON(r, &input, false); err != nil {
		s.respond(w, nil, err)
		return
	}
	req, err := input.request()
	if err != nil {
		s.respond(w, nil, err)
		return
	}
	v, err := s.manager.SubmitDecision(r.Context(), teamToken(r), req)
	s.respond(w, v, err)
}

func (s *serverAPI) closeRound(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Round *int
		Force bool
	}
	if err := readJSON(r, &input, false); err != nil {
		s.respond(w, nil, err)
		return
	}
	if input.Round == nil {
		s.respond(w, nil, badRequest("Укажите номер раунда"))
		return
	}
	v, err := s.manager.CloseRound(r.Context(), api.CloseRoundRequest{Round: *input.Round, Force: input.Force})
	s.respond(w, v, err)
}

func (s *serverAPI) applyEvents(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Round      *int
		Finish     bool
		Selections []struct {
			TeamID   *int64
			EventIDs []*int
		}
	}
	if err := readJSON(r, &input, false); err != nil {
		s.respond(w, nil, err)
		return
	}
	if input.Round == nil || input.Selections == nil {
		s.respond(w, nil, badRequest("Укажите номер раунда и выбор команд"))
		return
	}
	req := api.FireEventsRequest{Round: *input.Round, Finish: input.Finish}
	for _, sel := range input.Selections {
		if sel.TeamID == nil || sel.EventIDs == nil {
			s.respond(w, nil, badRequest("Укажите teamId и массив eventIds"))
			return
		}
		events, err := ids(sel.EventIDs)
		if err != nil {
			s.respond(w, nil, err)
			return
		}
		req.Selections = append(req.Selections, api.EventSelection{TeamID: *sel.TeamID, EventIDs: events})
	}
	v, err := s.manager.ApplyEvents(r.Context(), req)
	s.respond(w, v, err)
}
