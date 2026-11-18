package game

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Arseniy22877/game/internal/api"
)

func (m *Manager) Scenarios() api.ScenariosResponse {
	ids := m.catalog.IDs()
	out := api.ScenariosResponse{Scenarios: make([]api.Scenario, 0, len(ids))}
	for _, id := range ids {
		out.Scenarios = append(out.Scenarios, api.Scenario{ID: id})
	}
	return out
}

func (m *Manager) CreateLobby(ctx context.Context, confirm bool) (api.AdminState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.createGame(ctx, confirm); err != nil {
		return api.AdminState{}, err
	}
	return m.adminState(), nil
}

func (m *Manager) RegisterTeam(ctx context.Context, request api.RegisterTeamRequest) (api.RegisterTeamResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryErr != nil {
		return api.RegisterTeamResponse{}, m.recoveryErr
	}
	if m.info.Phase == api.PhaseNone {
		return api.RegisterTeamResponse{}, &Error{http.StatusConflict, api.CodeNoGame, "Активной игры нет"}
	}
	if err := m.checkLobby(); err != nil {
		return api.RegisterTeamResponse{}, err
	}
	name, members, err := normalizeTeam(request)
	if err != nil {
		return api.RegisterTeamResponse{}, err
	}
	nameKey := strings.ToLower(name)
	for _, t := range m.teams {
		if strings.ToLower(t.record.Name) == nameKey {
			return api.RegisterTeamResponse{}, &Error{http.StatusConflict, api.CodeNameTaken, "Команда с таким названием уже есть"}
		}
	}
	var tokenBytes [16]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return api.RegisterTeamResponse{}, err
	}
	token := hex.EncodeToString(tokenBytes[:])
	record, err := m.store.RegisterTeam(ctx, m.info.GameID, name, members, token)
	if err != nil {
		return api.RegisterTeamResponse{}, m.rollback(ctx, err)
	}
	t := team{record: record}
	m.teams = append(m.teams, t)
	m.notifyLobby()
	return api.RegisterTeamResponse{Token: token, Team: m.summary(t)}, nil
}

func normalizeTeam(request api.RegisterTeamRequest) (string, []string, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" || utf8.RuneCountInString(name) > 40 {
		return "", nil, &Error{http.StatusBadRequest, api.CodeBadRequest, "Название команды должно содержать от 1 до 40 символов"}
	}
	if len(request.Members) < 1 || len(request.Members) > 10 {
		return "", nil, &Error{http.StatusBadRequest, api.CodeBadRequest, "В команде должно быть от 1 до 10 участников"}
	}
	members := make([]string, len(request.Members))
	for i, member := range request.Members {
		member = strings.TrimSpace(member)
		if member == "" || utf8.RuneCountInString(member) > 60 {
			return "", nil, &Error{http.StatusBadRequest, api.CodeBadRequest, "Имя участника должно содержать от 1 до 60 символов"}
		}
		members[i] = member
	}
	return name, members, nil
}

func (m *Manager) SetReady(ctx context.Context, token string, ready bool) (api.TeamSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryErr != nil {
		return api.TeamSummary{}, m.recoveryErr
	}
	t := m.teamByToken(token)
	if t == nil {
		return api.TeamSummary{}, &Error{http.StatusUnauthorized, api.CodeUnauthorized, "Неизвестный токен команды"}
	}
	if err := m.checkLobby(); err != nil {
		return api.TeamSummary{}, err
	}
	if t.record.Ready == ready {
		return m.summary(*t), nil
	}
	if err := m.store.SetReady(ctx, m.info.GameID, t.record.ID, ready); err != nil {
		return api.TeamSummary{}, m.rollback(ctx, err)
	}
	t.record.Ready = ready
	m.notifyLobby()
	return m.summary(*t), nil
}

func (m *Manager) TeamID(token string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryErr != nil {
		return 0, m.recoveryErr
	}
	t := m.teamByToken(token)
	if t == nil {
		return 0, &Error{http.StatusUnauthorized, api.CodeUnauthorized, "Неизвестный токен команды"}
	}
	return t.record.ID, nil
}

func (m *Manager) DeleteTeam(ctx context.Context, id int64) (api.AdminState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkLobby(); err != nil {
		return api.AdminState{}, err
	}
	i := slices.IndexFunc(m.teams, func(t team) bool { return t.record.ID == id })
	if i == -1 {
		return api.AdminState{}, &Error{http.StatusNotFound, api.CodeNotFound, "Команда не найдена"}
	}
	if err := m.store.DeleteTeam(ctx, m.info.GameID, id); err != nil {
		return api.AdminState{}, m.rollback(ctx, err)
	}
	m.teams = slices.Delete(m.teams, i, i+1)
	m.notify.DropTeam(id)
	m.notifyLobby()
	return m.adminState(), nil
}

func (m *Manager) StartGame(ctx context.Context, id string) (api.AdminState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkLobby(); err != nil {
		return api.AdminState{}, err
	}
	if len(m.teams) == 0 || slices.ContainsFunc(m.teams, func(t team) bool { return !t.record.Ready }) {
		return api.AdminState{}, &Error{http.StatusConflict, api.CodeTeamsNotReady, "Для старта нужна хотя бы одна команда, все команды должны быть готовы"}
	}
	scenario, ok := m.catalog.Get(id)
	if !ok {
		return api.AdminState{}, &Error{http.StatusNotFound, api.CodeNotFound, "Сценарий не найден"}
	}
	teams := slices.Clone(m.teams)
	for i := range teams {
		t := &teams[i]
		t.instance = scenario.NewInstance()
		t.setRound(1)
	}
	if err := m.store.StartGame(ctx, m.info.GameID, id, scenario.Raw()); err != nil {
		return api.AdminState{}, m.rollback(ctx, err)
	}
	m.scenario = scenario
	m.teams = teams
	m.info.ScenarioID = id
	m.info.Phase = api.PhaseRoundOpen
	m.info.Round = 1
	m.phaseChanged()
	return m.adminState(), nil
}

func (m *Manager) checkLobby() error {
	if m.recoveryErr != nil {
		return m.recoveryErr
	}
	if m.info.Phase != api.PhaseLobby {
		return &Error{http.StatusConflict, api.CodeWrongPhase, "Действие доступно только в лобби"}
	}
	return nil
}

func (m *Manager) notifyLobby() {
	teams := make([]api.TeamSummary, 0, len(m.teams))
	for _, t := range m.teams {
		teams = append(teams, m.summary(t))
	}
	msg := api.WSMessage{Type: api.WSLobbyUpdated, Payload: api.LobbyUpdatedPayload{Teams: teams}}
	m.notify.ToAdmin(msg)
	m.notify.ToTeams(msg)
}
