package game

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/Arseniy22877/game/internal/enginebridge"
	"github.com/Arseniy22877/game/internal/store"
)

func (t *team) submitted() bool {
	return t.decision != nil && !t.decision.Auto
}

func (t *team) setRound(round int) {
	t.instance.SetRound(round)
	t.bankrupt = t.instance.Bankrupt()
	t.decision = nil
	t.predictions = nil
}

func (t *team) finish(round int) {
	t.setRound(round)
	t.bankrupt = t.lastRoundBankrupt
}

func (t *team) recordEvents(events api.RoundEvents) {
	t.lastRoundBankrupt = t.bankrupt
	t.history = append(t.history, events)
}

func (t *team) state() api.TeamGameState {
	dto := t.instance.State()
	for _, round := range t.history {
		dto.History = append(dto.History, api.RoundEvents{
			Round: round.Round, Events: append([]api.Event{}, round.Events...),
		})
	}
	return dto
}

func cloneDecision(d api.Decision) api.Decision {
	d.Attributes = append([]api.AttributeChoice{}, d.Attributes...)
	d.Strategies = append([]int{}, d.Strategies...)
	d.Hints = append([]int{}, d.Hints...)
	return d
}

func (m *Manager) checkRound(phase api.Phase, round int) error {
	if m.recoveryErr != nil {
		return m.recoveryErr
	}
	if m.info.Phase == api.PhaseNone {
		return &Error{http.StatusConflict, api.CodeNoGame, "Нет активной игры"}
	}
	if m.info.Phase != phase || m.info.Round != round {
		return &Error{http.StatusConflict, api.CodeWrongPhase, "Действие недоступно в текущем раунде"}
	}
	return nil
}

func (m *Manager) SubmitDecision(ctx context.Context, token string, req api.SubmitDecisionRequest) (api.SubmitDecisionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryErr != nil {
		return api.SubmitDecisionResponse{}, m.recoveryErr
	}
	t := m.teamByToken(token)
	if t == nil {
		return api.SubmitDecisionResponse{}, &Error{http.StatusUnauthorized, api.CodeUnauthorized, "Неизвестный токен команды"}
	}
	if err := m.checkRound(api.PhaseRoundOpen, req.Round); err != nil {
		return api.SubmitDecisionResponse{}, err
	}
	if t.bankrupt {
		return api.SubmitDecisionResponse{}, &Error{http.StatusConflict, api.CodeTeamBankrupt, "Команда пропускает этот раунд"}
	}

	d := cloneDecision(req.Decision)
	for _, answer := range []*string{&d.Report.Why, &d.Report.Alternative, &d.Report.Risks, &d.Report.Correction} {
		*answer = strings.TrimSpace(*answer)
		if utf8.RuneCountInString(*answer) > 4000 {
			return api.SubmitDecisionResponse{}, &Error{http.StatusBadRequest, api.CodeBadRequest, "Ответ в отчёте должен содержать не более 4000 символов"}
		}
	}
	cost, err := t.instance.Submit(d)
	if err != nil {
		var invalid *enginebridge.DecisionError
		if errors.As(err, &invalid) {
			return api.SubmitDecisionResponse{}, &Error{http.StatusUnprocessableEntity, invalid.Code, invalid.Message}
		}
		return api.SubmitDecisionResponse{}, m.rollback(ctx, err)
	}
	record := store.DecisionRecord{Decision: d, Cost: cost}
	if err := m.store.SaveDecision(ctx, m.info.GameID, t.record.ID, req.Round, record); err != nil {
		return api.SubmitDecisionResponse{}, m.rollback(ctx, fmt.Errorf("save decision for team %d: %w", t.record.ID, err))
	}
	t.decision = &record
	msg := api.WSMessage{Type: api.WSSubmissionUpdated, Payload: api.SubmissionUpdatedPayload{TeamID: t.record.ID, Submitted: true}}
	m.notify.ToAdmin(msg)
	m.notify.ToTeam(t.record.ID, msg)
	return api.SubmitDecisionResponse{Cost: cost}, nil
}

func (m *Manager) CloseRound(ctx context.Context, req api.CloseRoundRequest) (api.AdminState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkRound(api.PhaseRoundOpen, req.Round); err != nil {
		return api.AdminState{}, err
	}
	if !req.Force {
		for _, t := range m.teams {
			if !t.bankrupt && !t.submitted() {
				return api.AdminState{}, &Error{http.StatusConflict, api.CodeNotAllSubmitted, "Не все команды отправили решения"}
			}
		}
	}

	calculations := make([]store.RoundCalculation, 0, len(m.teams))
	for i := range m.teams {
		t := &m.teams[i]
		calc := store.RoundCalculation{TeamID: t.record.ID}
		if err := t.calculate(&calc); err != nil {
			return api.AdminState{}, m.rollback(ctx, fmt.Errorf("calculate round %d for team %d: %w", req.Round, t.record.ID, err))
		}
		calculations = append(calculations, calc)
	}
	if err := m.store.CloseRound(ctx, m.info.GameID, req.Round, calculations); err != nil {
		return api.AdminState{}, m.rollback(ctx, fmt.Errorf("save round %d: %w", req.Round, err))
	}
	m.info.Phase = api.PhaseRoundClosed
	m.phaseChanged()
	return m.adminState(), nil
}

func (t *team) calculate(calc *store.RoundCalculation) error {
	if t.bankrupt {
		if _, err := t.instance.SkipRound(); err != nil {
			return err
		}
	} else {
		if t.decision == nil {
			d := t.instance.DefaultDecision()
			cost, err := t.instance.Submit(d)
			if err != nil {
				return err
			}
			t.decision = &store.DecisionRecord{Decision: d, Cost: cost, Auto: true}
			calc.AutoDecision = t.decision
		}
		if _, err := t.instance.Calculate(); err != nil {
			return err
		}
	}
	t.predictions = t.instance.Suggest()
	calc.Predictions = t.predictions
	calc.State = t.state()
	return nil
}

type validatedSelection struct {
	team   *team
	ids    []int
	events api.RoundEvents
}

func (m *Manager) validateSelections(req api.FireEventsRequest) ([]validatedSelection, error) {
	teams := make(map[int64]*team, len(m.teams))
	for i := range m.teams {
		t := &m.teams[i]
		teams[t.record.ID] = t
	}
	selections := make([]validatedSelection, 0, len(req.Selections))
	for _, s := range req.Selections {
		t, ok := teams[s.TeamID]
		if !ok {
			return nil, &Error{http.StatusBadRequest, api.CodeBadRequest, fmt.Sprintf("Команда %d указана дважды или отсутствует в игре", s.TeamID)}
		}
		delete(teams, s.TeamID)
		ids := append([]int{}, s.EventIDs...)
		result := store.RoundResult{FiredEventIDs: ids, Predictions: t.predictions}
		events, err := result.Events()
		if err != nil {
			return nil, &Error{http.StatusUnprocessableEntity, api.CodeInvalidEvent, fmt.Sprintf("Выбор событий для команды «%s» содержит повтор или событие вне прогноза", t.record.Name)}
		}
		selections = append(selections, validatedSelection{
			team: t, ids: ids, events: api.RoundEvents{Round: req.Round, Events: events},
		})
	}
	if len(teams) != 0 {
		return nil, &Error{http.StatusConflict, api.CodeNotAllTeams, "Выберите события для всех команд"}
	}
	return selections, nil
}

func (m *Manager) ApplyEvents(ctx context.Context, req api.FireEventsRequest) (api.AdminState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkRound(api.PhaseRoundClosed, req.Round); err != nil {
		return api.AdminState{}, err
	}
	selections, err := m.validateSelections(req)
	if err != nil {
		return api.AdminState{}, err
	}

	records := make([]store.RoundEventSelection, 0, len(selections))
	for _, s := range selections {
		if err := s.team.instance.Fire(s.ids); err != nil {
			return api.AdminState{}, m.rollback(ctx, fmt.Errorf("fire events in round %d for team %d: %w", req.Round, s.team.record.ID, err))
		}
		s.team.recordEvents(s.events)
		records = append(records, store.RoundEventSelection{
			TeamID: s.team.record.ID, EventIDs: s.ids, State: s.team.state(),
		})
	}
	if err := m.store.ApplyEvents(ctx, m.info.GameID, req.Round, records, req.Finish); err != nil {
		return api.AdminState{}, m.rollback(ctx, fmt.Errorf("save events in round %d: %w", req.Round, err))
	}

	if req.Finish {
		m.setRound(api.PhaseFinished, req.Round)
	} else {
		m.setRound(api.PhaseRoundOpen, req.Round+1)
	}
	for _, s := range selections {
		m.notify.ToTeam(s.team.record.ID, api.WSMessage{Type: api.WSTeamState, Payload: s.events})
	}
	m.phaseChanged()
	return m.adminState(), nil
}

func (m *Manager) Finish(ctx context.Context) (api.AdminState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkRound(api.PhaseRoundOpen, m.info.Round); err != nil {
		return api.AdminState{}, err
	}
	if err := m.store.FinishGame(ctx, m.info.GameID, m.info.Round); err != nil {
		return api.AdminState{}, m.rollback(ctx, fmt.Errorf("finish game %d: %w", m.info.GameID, err))
	}
	m.setRound(api.PhaseFinished, m.info.Round-1)
	m.phaseChanged()
	return m.adminState(), nil
}

func (m *Manager) setRound(phase api.Phase, round int) {
	for i := range m.teams {
		t := &m.teams[i]
		if phase == api.PhaseFinished {
			t.finish(round)
		} else {
			t.setRound(round)
		}
	}
	m.info.Phase = phase
	m.info.Round = round
}

func (m *Manager) phaseChanged() {
	m.notify.ToAll(api.WSMessage{Type: api.WSPhaseChanged, Payload: m.info})
}
