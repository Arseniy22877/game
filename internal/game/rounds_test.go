package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/Arseniy22877/game/internal/enginebridge"
	"github.com/Arseniy22877/game/internal/store"
)

type roundNotice struct {
	audience string
	teamID   int64
	message  api.WSMessage
}

type roundNotifier struct {
	notices []roundNotice
}

func (n *roundNotifier) ToAll(msg api.WSMessage)   { n.add("all", 0, msg) }
func (n *roundNotifier) ToAdmin(msg api.WSMessage) { n.add("admin", 0, msg) }
func (n *roundNotifier) ToTeams(msg api.WSMessage) { n.add("teams", 0, msg) }
func (n *roundNotifier) ToTeam(id int64, msg api.WSMessage) {
	n.add("team", id, msg)
}
func (n *roundNotifier) add(audience string, id int64, msg api.WSMessage) {
	n.notices = append(n.notices, roundNotice{audience: audience, teamID: id, message: msg})
}
func (*roundNotifier) Online(int64) bool { return false }
func (*roundNotifier) DropTeam(int64)    {}
func (*roundNotifier) DropAllTeams()     {}

type roundSession struct {
	manager *Manager
	db      *store.Store
	catalog *enginebridge.Catalog
	notify  *roundNotifier
	path    string
	teams   []api.RegisterTeamResponse
}

func newRoundSession(t *testing.T) *roundSession {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	raw, err := os.ReadFile("../enginebridge/testdata/full.json")
	if err != nil {
		t.Fatal(err)
	}
	var scenario map[string]json.RawMessage
	if err := json.Unmarshal(raw, &scenario); err != nil {
		t.Fatal(err)
	}
	var events []map[string]any
	if err := json.Unmarshal(scenario["Events"], &events); err != nil {
		t.Fatal(err)
	}
	events[1]["BudgetChange"] = map[string]int{"MoneyShift": 8}
	events[2]["BudgetChange"] = map[string]int{"MoneyShift": -10}
	events[5]["BudgetChange"] = map[string]int{"MoneyShift": 15}
	scenario["Events"], err = json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(scenario)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credit.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := enginebridge.LoadCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "game.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	n := &roundNotifier{}
	m, err := NewManager(ctx, db, catalog, n)
	if err != nil {
		t.Fatal(err)
	}
	s := &roundSession{manager: m, db: db, catalog: catalog, notify: n, path: path}
	if _, err := m.CreateLobby(ctx, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Первая", "Вторая"} {
		team, err := m.RegisterTeam(ctx, api.RegisterTeamRequest{Name: name, Members: []string{"Участник"}})
		if err != nil {
			t.Fatal(err)
		}
		s.teams = append(s.teams, team)
	}
	if _, err := m.StartGame(ctx, "credit"); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s *roundSession) assertReplay(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	fresh, err := NewManager(ctx, s.db, s.catalog, &roundNotifier{})
	if err != nil {
		t.Fatal(err)
	}
	if want, got := s.manager.AdminState(), fresh.AdminState(); !reflect.DeepEqual(want, got) {
		t.Fatalf("admin state differs after restart: %+v, want %+v", got, want)
	}
	for _, team := range s.teams {
		want, err := s.manager.TeamState(team.Token)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := fresh.TeamState(team.Token); err != nil || !reflect.DeepEqual(want, got) {
			t.Fatalf("team state differs after restart: %+v, want %+v, error: %v", got, want, err)
		}
	}
}

func assertRoundError(t *testing.T, err error, code string) {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatalf("got %v, want %s", err, code)
	}
}

func decisionAt(round int, value float64) api.SubmitDecisionRequest {
	return api.SubmitDecisionRequest{
		Round: round,
		Decision: api.Decision{
			Attributes: []api.AttributeChoice{{ID: 1, Value: value}},
			Report:     api.Report{Why: "  Пояснение  "},
		},
	}
}

func (s *roundSession) assertRollback(t *testing.T, trigger string, action func() error) {
	t.Helper()
	db, err := sql.Open("sqlite", s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	before := s.manager.AdminState()
	notifications := len(s.notify.notices)
	if err := action(); err == nil {
		t.Fatal("expected injected database failure")
	}
	if !reflect.DeepEqual(before, s.manager.AdminState()) || notifications != len(s.notify.notices) {
		t.Fatal("failed transaction changed state or notified clients")
	}
	s.assertReplay(t)
	if _, err := db.Exec("DROP TRIGGER fail_round"); err != nil {
		t.Fatal(err)
	}
}

func TestRoundLifecycleAndRollback(t *testing.T) {
	ctx := context.Background()
	s := newRoundSession(t)
	m := s.manager
	first, second := s.teams[0], s.teams[1]
	s.assertReplay(t)

	if _, err := m.SubmitDecision(ctx, first.Token, decisionAt(1, 4)); err != nil {
		t.Fatal(err)
	}
	s.assertReplay(t)
	startNotices := len(s.notify.notices)
	result, err := m.SubmitDecision(ctx, first.Token, decisionAt(1, 10))
	if err != nil || result.Cost.Money != 15 {
		t.Fatal(result, err)
	}
	submitted := api.WSMessage{Type: api.WSSubmissionUpdated, Payload: api.SubmissionUpdatedPayload{TeamID: first.Team.ID, Submitted: true}}
	if got, want := s.notify.notices[startNotices:], []roundNotice{{"admin", 0, submitted}, {"team", first.Team.ID, submitted}}; !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	s.assertReplay(t)
	s.assertRollback(t, fmt.Sprintf("CREATE TRIGGER fail_round BEFORE UPDATE ON decisions WHEN NEW.team_id = %d BEGIN SELECT RAISE(ABORT, 'injected'); END", first.Team.ID), func() error {
		_, err := m.SubmitDecision(ctx, first.Token, decisionAt(1, 5))
		return err
	})

	invalid := decisionAt(1, 10)
	invalid.Report.Why = strings.Repeat("я", 4001)
	_, err = m.SubmitDecision(ctx, first.Token, invalid)
	assertRoundError(t, err, api.CodeBadRequest)
	_, err = m.SubmitDecision(ctx, first.Token, decisionAt(1, 11))
	assertRoundError(t, err, api.CodeInvalidDecision)
	_, err = m.SubmitDecision(ctx, first.Token, decisionAt(2, 4))
	assertRoundError(t, err, api.CodeWrongPhase)
	_, err = m.CloseRound(ctx, api.CloseRoundRequest{Round: 1})
	assertRoundError(t, err, api.CodeNotAllSubmitted)
	if v := m.AdminState().Teams[0]; v.Decision.Report.Why != "Пояснение" || v.Decision.Attributes[0].Value != 10 || v.State.Budget.Money != 17 {
		t.Fatal(v)
	}

	closeFirst := func() (api.AdminState, error) {
		return m.CloseRound(ctx, api.CloseRoundRequest{Round: 1, Force: true})
	}
	s.assertRollback(t, fmt.Sprintf("CREATE TRIGGER fail_round BEFORE INSERT ON round_results WHEN NEW.team_id = %d BEGIN SELECT RAISE(ABORT, 'injected'); END", second.Team.ID), func() error {
		_, err := closeFirst()
		return err
	})
	closed, err := closeFirst()
	if err != nil {
		t.Fatal(err)
	}
	a, b := closed.Teams[0], closed.Teams[1]
	if a.Bankrupt || !a.Submitted || a.State.Budget.Money != 2 || a.Decision == nil || b.Submitted || b.Decision == nil || b.Cost.Money != 9 {
		t.Fatal(a, b)
	}
	s.assertReplay(t)
	if !reflect.DeepEqual(closed, m.AdminState()) {
		t.Fatal("GET changed the closed-round predictions")
	}
	_, err = m.Finish(ctx)
	assertRoundError(t, err, api.CodeWrongPhase)

	selection := api.FireEventsRequest{Round: 1, Selections: []api.EventSelection{{TeamID: first.Team.ID, EventIDs: []int{}}, {TeamID: second.Team.ID, EventIDs: []int{2, 1}}}}
	_, err = m.ApplyEvents(ctx, api.FireEventsRequest{Round: 1, Selections: selection.Selections[:1]})
	assertRoundError(t, err, api.CodeNotAllTeams)
	_, err = m.ApplyEvents(ctx, api.FireEventsRequest{Round: 1, Selections: []api.EventSelection{selection.Selections[0], selection.Selections[0]}})
	assertRoundError(t, err, api.CodeBadRequest)
	_, err = m.ApplyEvents(ctx, api.FireEventsRequest{Round: 1, Selections: []api.EventSelection{{TeamID: first.Team.ID, EventIDs: []int{2}}, {TeamID: second.Team.ID, EventIDs: []int{2, 2}}}})
	assertRoundError(t, err, api.CodeInvalidEvent)
	if !reflect.DeepEqual(closed, m.AdminState()) {
		t.Fatal("rejected events changed round state")
	}
	s.assertRollback(t, fmt.Sprintf("CREATE TRIGGER fail_round BEFORE UPDATE ON round_results WHEN NEW.team_id = %d BEGIN SELECT RAISE(ABORT, 'injected'); END", second.Team.ID), func() error {
		_, err := m.ApplyEvents(ctx, selection)
		return err
	})

	startNotices = len(s.notify.notices)
	next, err := m.ApplyEvents(ctx, selection)
	if err != nil || next.Game.Round != 2 {
		t.Fatal(next, err)
	}
	a, b = next.Teams[0], next.Teams[1]
	firstRound, secondRound := a.State.History[0], b.State.History[0]
	if !a.Bankrupt || a.Decision != nil || b.Submitted || b.Bankrupt || b.State.Budget.Money != 8 || firstRound.Events == nil || len(firstRound.Events) != 0 || len(secondRound.Events) != 2 || secondRound.Events[0].ID != 2 || secondRound.Events[1].ID != 1 {
		t.Fatal(a, b)
	}
	if got, want := s.notify.notices[startNotices:], []roundNotice{
		{"team", first.Team.ID, api.WSMessage{Type: api.WSTeamState, Payload: firstRound}},
		{"team", second.Team.ID, api.WSMessage{Type: api.WSTeamState, Payload: secondRound}},
		{"all", 0, api.WSMessage{Type: api.WSPhaseChanged, Payload: next.Game}},
	}; !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	s.assertReplay(t)
	_, err = m.ApplyEvents(ctx, selection)
	assertRoundError(t, err, api.CodeWrongPhase)
	_, err = m.SubmitDecision(ctx, first.Token, decisionAt(2, 1))
	assertRoundError(t, err, api.CodeTeamBankrupt)
	if _, err := m.SubmitDecision(ctx, second.Token, decisionAt(2, 1)); err != nil {
		t.Fatal(err)
	}
	s.assertReplay(t)

	closed, err = m.CloseRound(ctx, api.CloseRoundRequest{Round: 2})
	if err != nil || closed.Teams[0].State.Budget.Money != 2 || !slices.ContainsFunc(closed.Teams[0].Predictions, func(p api.Prediction) bool { return p.Event.ID == 5 }) {
		t.Fatal(closed, err)
	}
	s.assertReplay(t)
	selection.Round = 2
	selection.Selections[0].EventIDs = []int{5}
	selection.Selections[1].EventIDs = []int{}
	next, err = m.ApplyEvents(ctx, selection)
	if err != nil || next.Game.Round != 3 || next.Teams[0].Bankrupt || next.Teams[0].State.Budget.Money != 17 || !next.Teams[1].Bankrupt {
		t.Fatal(next, err)
	}
	s.assertReplay(t)

	if _, err := m.CloseRound(ctx, api.CloseRoundRequest{Round: 3, Force: true}); err != nil {
		t.Fatal(err)
	}
	s.assertReplay(t)
	selection.Round, selection.Finish = 3, true
	selection.Selections[0].EventIDs = []int{3}
	final, err := m.ApplyEvents(ctx, selection)
	if err != nil || final.Game.Phase != api.PhaseFinished || final.Game.Round != 3 ||
		final.Teams[0].Decision != nil || final.Teams[0].Bankrupt || !final.Teams[1].Bankrupt {
		t.Fatal(final, err)
	}
	if events := final.Teams[0].State.History[2].Events; len(events) != 1 || events[0].ID != 3 {
		t.Fatal(events)
	}
	s.assertReplay(t)
}

func TestFinishOpenRoundDiscardsPendingDecision(t *testing.T) {
	ctx := context.Background()
	s := newRoundSession(t)
	if _, err := s.manager.SubmitDecision(ctx, s.teams[0].Token, decisionAt(1, 10)); err != nil {
		t.Fatal(err)
	}
	s.assertReplay(t)
	final, err := s.manager.Finish(ctx)
	if err != nil || final.Game.Phase != api.PhaseFinished || final.Game.Round != 0 {
		t.Fatal(final, err)
	}
	team := final.Teams[0]
	if team.Decision != nil || team.Submitted || team.State.Budget.Money != 17 || len(team.State.History) != 0 {
		t.Fatal(team)
	}
	s.assertReplay(t)
}
