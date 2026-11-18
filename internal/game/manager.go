package game

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
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
	record            store.Team
	instance          *enginebridge.Instance
	bankrupt          bool
	lastRoundBankrupt bool
	decision          *store.DecisionRecord
	predictions       []api.Prediction
	history           []api.RoundEvents
}

type Manager struct {
	mu          sync.Mutex
	store       *store.Store
	catalog     *enginebridge.Catalog
	notify      Notifier
	info        api.GameInfo
	teams       []team
	scenario    *enginebridge.Scenario
	recoveryErr error
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

func (m *Manager) rollback(ctx context.Context, cause error) error {
	m.recoveryErr = m.rebuild(context.WithoutCancel(ctx))
	return errors.Join(cause, m.recoveryErr)
}

func gameInfo(g store.Game) api.GameInfo {
	return api.GameInfo{GameID: g.ID, Phase: g.Phase, Round: g.Round, ScenarioID: g.ScenarioID, BaseRoundCost: enginebridge.BaseRoundCost}
}

func (m *Manager) CreateGame(ctx context.Context, confirm bool) (api.GameInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createGame(ctx, confirm)
}

func (m *Manager) createGame(ctx context.Context, confirm bool) (api.GameInfo, error) {
	if m.recoveryErr != nil {
		return api.GameInfo{}, m.recoveryErr
	}
	if !confirm && (m.info.Phase == api.PhaseRoundOpen || m.info.Phase == api.PhaseRoundClosed) {
		return api.GameInfo{}, &Error{409, api.CodeGameInProgress, "Подтвердите завершение текущей игры"}
	}
	g, err := m.store.CreateGame(ctx)
	if err != nil {
		return api.GameInfo{}, m.rollback(ctx, err)
	}
	m.info = gameInfo(g)
	m.teams = nil
	m.scenario = nil
	m.phaseChanged()
	m.notify.DropAllTeams()
	return m.info, nil
}

func (m *Manager) GameInfo() api.GameInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.info
}

func (m *Manager) ReadGameInfo() (api.GameInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.info, m.recoveryErr
}

func (m *Manager) summary(t team) api.TeamSummary {
	r := t.record
	return api.TeamSummary{
		ID: r.ID, Name: r.Name, Members: append([]string{}, r.Members...), Ready: r.Ready,
		Online: m.notify.Online(r.ID), Bankrupt: t.bankrupt, Submitted: t.submitted(),
	}
}

func (m *Manager) AdminState() api.AdminState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.adminState()
}

func (m *Manager) ReadAdminState() (api.AdminState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryErr != nil {
		return api.AdminState{}, m.recoveryErr
	}
	return m.adminState(), nil
}

func (m *Manager) adminState() api.AdminState {
	dto := api.AdminState{Game: m.info, Teams: []api.AdminTeam{}}
	for _, t := range m.teams {
		v := api.AdminTeam{TeamSummary: m.summary(t), Token: t.record.Token, Predictions: append([]api.Prediction{}, t.predictions...)}
		if t.instance != nil {
			state := t.state()
			v.State = &state
		}
		if t.decision != nil {
			decision, cost := cloneDecision(t.decision.Decision), t.decision.Cost
			v.Decision = &decision
			v.Cost = &cost
		}
		dto.Teams = append(dto.Teams, v)
	}
	return dto
}

func (m *Manager) TeamState(token string) (api.TeamStateResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryErr != nil {
		return api.TeamStateResponse{}, m.recoveryErr
	}
	t := m.teamByToken(token)
	if t == nil {
		return api.TeamStateResponse{}, &Error{http.StatusUnauthorized, api.CodeUnauthorized, "Неизвестный токен команды"}
	}
	dto := api.TeamStateResponse{Game: m.info, Team: m.summary(*t)}
	if t.instance != nil {
		state := t.state()
		dto.State = &state
		dto.Opening = m.scenario.Opening()
	}
	if t.decision != nil {
		decision := cloneDecision(t.decision.Decision)
		dto.Decision = &decision
	}
	return dto, nil
}

func (m *Manager) teamByToken(token string) *team {
	if token == "" {
		return nil
	}
	for i := range m.teams {
		if m.teams[i].record.Token == token {
			return &m.teams[i]
		}
	}
	return nil
}

func (m *Manager) Presence(id int64, online bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryErr != nil || !slices.ContainsFunc(m.teams, func(t team) bool { return t.record.ID == id }) {
		return
	}
	if m.info.Phase == api.PhaseLobby {
		m.notifyLobby()
		return
	}
	m.notify.ToAdmin(api.WSMessage{Type: api.WSTeamOnline, Payload: api.TeamOnlinePayload{TeamID: id, Online: online}})
}
