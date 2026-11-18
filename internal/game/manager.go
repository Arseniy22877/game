package game

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/Arseniy22877/game/internal/enginebridge"
	"github.com/Arseniy22877/game/internal/store"
)

type Notifier interface {
	ToAll(api.WSMessage)
	ToAdmin(api.WSMessage)
	ToTeams(api.WSMessage)
	ToTeam(int64, api.WSMessage)
	Online(int64) bool
	DropTeam(int64)
	DropAllTeams()
}

type Error struct {
	Status        int
	Code, Message string
}

func (e *Error) Error() string { return e.Message }

type team struct {
	record   store.Team
	instance *enginebridge.Instance
}

type Manager struct {
	mu       sync.Mutex
	store    *store.Store
	catalog  *enginebridge.Catalog
	notify   Notifier
	info     api.GameInfo
	teams    []team
	scenario *enginebridge.Scenario
}

func NewManager(ctx context.Context, s *store.Store, catalog *enginebridge.Catalog, n Notifier) (*Manager, error) {
	m := &Manager{store: s, catalog: catalog, notify: n}
	if err := m.rebuild(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) rebuild(ctx context.Context) error {
	j, err := m.store.ActiveJournal(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		m.info = api.GameInfo{Phase: api.PhaseNone, BaseRoundCost: enginebridge.BaseRoundCost}
		m.teams = nil
		m.scenario = nil
		return nil
	}
	if err != nil {
		return err
	}
	return m.replay(j)
}

func (m *Manager) replay(j store.Journal) error {
	g := j.Game
	var scenario *enginebridge.Scenario
	switch g.Phase {
	case api.PhaseLobby:
	case api.PhaseRoundOpen, api.PhaseRoundClosed, api.PhaseFinished:
		var err error
		scenario, err = m.catalog.ParseSnapshot(g.ScenarioID, g.ScenarioJSON)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown phase %q", g.Phase)
	}
	teams := make([]team, 0, len(j.Teams))
	for _, record := range j.Teams {
		if len(record.Rounds) > 0 {
			return fmt.Errorf("round journal replay is unavailable for team %d", record.ID)
		}
		t := team{record: record.Team}
		if scenario != nil {
			t.instance = scenario.NewInstance()
			t.instance.SetRound(g.Round)
		}
		teams = append(teams, t)
	}
	m.teams = teams
	m.scenario = scenario
	m.info = gameInfo(g)
	return nil
}

func gameInfo(g store.Game) api.GameInfo {
	return api.GameInfo{GameID: g.ID, Phase: g.Phase, Round: g.Round, ScenarioID: g.ScenarioID, BaseRoundCost: enginebridge.BaseRoundCost}
}

func (m *Manager) CreateGame(ctx context.Context, confirm bool) (api.GameInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !confirm && (m.info.Phase == api.PhaseRoundOpen || m.info.Phase == api.PhaseRoundClosed) {
		return api.GameInfo{}, &Error{409, api.CodeGameInProgress, "Подтвердите завершение текущей игры"}
	}
	g, err := m.store.CreateGame(ctx)
	if err != nil {
		return api.GameInfo{}, errors.Join(err, m.rebuild(context.WithoutCancel(ctx)))
	}
	m.info = gameInfo(g)
	m.teams = nil
	m.scenario = nil
	m.notify.ToAll(api.WSMessage{Type: api.WSPhaseChanged, Payload: m.info})
	m.notify.DropAllTeams()
	return m.info, nil
}

func (m *Manager) GameInfo() api.GameInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.info
}
func (m *Manager) summary(t team) api.TeamSummary {
	r := t.record
	return api.TeamSummary{ID: r.ID, Name: r.Name, Members: append([]string{}, r.Members...), Ready: r.Ready, Online: m.notify.Online(r.ID), Bankrupt: t.instance != nil && t.instance.Bankrupt()}
}

func (m *Manager) AdminState() api.AdminState {
	m.mu.Lock()
	defer m.mu.Unlock()
	dto := api.AdminState{Game: m.info, Teams: []api.AdminTeam{}}
	for _, t := range m.teams {
		v := api.AdminTeam{TeamSummary: m.summary(t), Token: t.record.Token, Predictions: []api.Prediction{}}
		if t.instance != nil {
			state := t.instance.State()
			v.State = &state
		}
		dto.Teams = append(dto.Teams, v)
	}
	return dto
}

func (m *Manager) TeamState(token string) (api.TeamStateResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.teams {
		if t.record.Token == token {
			dto := api.TeamStateResponse{Game: m.info, Team: m.summary(t)}
			if t.instance != nil {
				state := t.instance.State()
				dto.State = &state
				dto.Opening = m.scenario.Opening()
			}
			return dto, nil
		}
	}
	return api.TeamStateResponse{}, &Error{401, api.CodeUnauthorized, "Неизвестный токен команды"}
}

func (m *Manager) Presence(id int64, online bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !slices.ContainsFunc(m.teams, func(t team) bool { return t.record.ID == id }) {
		return
	}
	if m.info.Phase == api.PhaseLobby {
		teams := []api.TeamSummary{}
		for _, t := range m.teams {
			teams = append(teams, m.summary(t))
		}
		msg := api.WSMessage{Type: api.WSLobbyUpdated, Payload: api.LobbyUpdatedPayload{Teams: teams}}
		m.notify.ToAdmin(msg)
		m.notify.ToTeams(msg)
		return
	}
	m.notify.ToAdmin(api.WSMessage{Type: api.WSTeamOnline, Payload: api.TeamOnlinePayload{TeamID: id, Online: online}})
}
