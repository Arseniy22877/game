package store

import (
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/Arseniy22877/game/internal/api"
)

func requireOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(t.Context(), path)
	requireOK(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func createGame(t *testing.T, s *Store) Game {
	t.Helper()
	game, err := s.CreateGame(t.Context())
	requireOK(t, err)
	return game
}

func registerTeam(t *testing.T, s *Store, gameID int64, name, token string) Team {
	t.Helper()
	team, err := s.RegisterTeam(t.Context(), gameID, name, []string{"Аня", "Петя"}, token)
	requireOK(t, err)
	return team
}

func activeJournal(t *testing.T, s *Store) Journal {
	t.Helper()
	journal, err := s.ActiveJournal(t.Context())
	requireOK(t, err)
	return journal
}

func gameDetail(t *testing.T, s *Store, gameID int64) api.GameDetail {
	t.Helper()
	detail, err := s.GameDetail(t.Context(), gameID)
	requireOK(t, err)
	return detail
}

func TestOpenAndMigrate(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "game?#%.db")
	s := openTestStore(t, path)
	if _, err := s.ActiveJournal(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("empty database: got %v, want sql.ErrNoRows", err)
	}
	game := createGame(t, s)
	requireOK(t, s.Close())

	s = openTestStore(t, path)
	if got := activeJournal(t, s).Game; !reflect.DeepEqual(got, game) {
		t.Fatalf("reopened game = %+v, want %+v", got, game)
	}
	for pragma, want := range map[string]string{
		"user_version": "1", "journal_mode": "wal", "synchronous": "2", "foreign_keys": "1", "busy_timeout": "5000",
	} {
		var got string
		requireOK(t, s.db.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&got))
		if got != want {
			t.Errorf("PRAGMA %s = %q, want %q", pragma, got, want)
		}
	}
	if got := s.db.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("max open connections = %d, want 1", got)
	}

	files := fstest.MapFS{
		"0001_init.sql":  {Data: []byte("THIS MUST BE SKIPPED")},
		"0002_extra.sql": {Data: []byte("CREATE TABLE extra (id INTEGER); BROKEN SQL;")},
	}
	if err := migrate(ctx, s.db, files); err == nil {
		t.Fatal("invalid migration succeeded")
	}
	var version, tables int
	requireOK(t, s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version))
	requireOK(t, s.db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name = 'extra'").Scan(&tables))
	if version != 1 || tables != 0 {
		t.Fatalf("failed migration left version %d and %d new tables", version, tables)
	}
	files["0002_extra.sql"].Data = []byte("CREATE TABLE extra (id INTEGER);")
	requireOK(t, migrate(ctx, s.db, files))
	requireOK(t, migrate(ctx, s.db, files))
	requireOK(t, s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version))
	if version != 2 {
		t.Errorf("retried migration left version %d, want 2", version)
	}
}

func TestLobbyAndArchive(t *testing.T) {
	ctx := t.Context()
	s := openTestStore(t, filepath.Join(t.TempDir(), "game.db"))
	game := createGame(t, s)
	team := registerTeam(t, s, game.ID, " Ромашка ", "first")
	removed := registerTeam(t, s, game.ID, "Лютик", "second")
	requireOK(t, s.SetReady(ctx, game.ID, team.ID, false))
	requireOK(t, s.DeleteTeam(ctx, game.ID, removed.ID))
	if _, err := s.RegisterTeam(ctx, game.ID, "РОМАШКА", nil, "third"); err == nil {
		t.Fatal("duplicate normalized name was accepted")
	}
	if _, err := s.db.ExecContext(ctx, "INSERT INTO games (phase, created_at) VALUES ('lobby', ?)", now()); err == nil {
		t.Fatal("database allowed two active games")
	}
	journal := activeJournal(t, s)
	if len(journal.Teams) != 1 {
		t.Fatalf("lobby teams = %+v, want only the remaining team", journal.Teams)
	}
	team.Ready = false
	if got := journal.Teams[0].Team; !reflect.DeepEqual(got, team) || got.Name != "Ромашка" {
		t.Errorf("lobby team = %+v, want %+v with trimmed name", got, team)
	}

	_, err := s.db.ExecContext(ctx, `
		CREATE TRIGGER reject_new_game BEFORE INSERT ON games
		BEGIN SELECT RAISE(ABORT, 'test insert failure'); END`)
	requireOK(t, err)
	if _, err := s.CreateGame(ctx); err == nil {
		t.Fatal("create game succeeded despite insert failure")
	}
	if got := activeJournal(t, s).Game.ID; got != game.ID {
		t.Errorf("failed replacement changed active game to %d", got)
	}
	_, err = s.db.ExecContext(ctx, "DROP TRIGGER reject_new_game")
	requireOK(t, err)
	newGame := createGame(t, s)
	registerTeam(t, s, newGame.ID, team.Name, "third")
	if err := s.SetReady(ctx, newGame.ID, team.ID, true); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("changing readiness across games: got %v, want sql.ErrNoRows", err)
	}
	games, err := s.Games(ctx)
	requireOK(t, err)
	if len(games) != 2 || games[0].GameID != newGame.ID || !games[0].Active || games[1].GameID != game.ID || games[1].Active {
		t.Errorf("game history after replacement = %+v", games)
	}
	if got := gameDetail(t, s, game.ID).Teams; len(got) != 1 || got[0].ID != team.ID {
		t.Errorf("archived teams = %+v", got)
	}
}

func TestRoundJournalAndHistory(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "game.db")
	s := openTestStore(t, path)
	game := createGame(t, s)
	first := registerTeam(t, s, game.ID, "Первая", "first")
	second := registerTeam(t, s, game.ID, "Вторая", "second")
	raw := []byte("{ \"scenario\": 1 }\n")
	requireOK(t, s.StartGame(ctx, game.ID, "scenario", raw))

	saved := DecisionRecord{
		Decision: api.Decision{
			Attributes: []api.AttributeChoice{{ID: 3, Value: 2.5}},
			Strategies: []int{8}, Hints: []int{9}, Report: api.Report{Why: "повторная отправка"},
		},
		Cost: api.Cost{Money: 12, QP: 5},
	}
	requireOK(t, s.SaveDecision(ctx, game.ID, first.ID, 1, DecisionRecord{}))
	requireOK(t, s.SaveDecision(ctx, game.ID, first.ID, 1, saved))
	requireOK(t, s.Close())
	s = openTestStore(t, path)
	journal := activeJournal(t, s)
	if journal.Game.Phase != api.PhaseRoundOpen || journal.Game.Round != 1 ||
		journal.Game.ScenarioID != "scenario" || !slices.Equal(journal.Game.ScenarioJSON, raw) {
		t.Fatalf("reopened game = %+v", journal.Game)
	}
	rounds := journal.Teams[0].Rounds
	if len(rounds) != 1 || !reflect.DeepEqual(rounds[0].Decision, &saved) ||
		rounds[0].Result != nil || len(journal.Teams[1].Rounds) != 0 {
		t.Fatalf("unexpected submissions: %+v", journal.Teams)
	}

	zero := api.Event{ID: 0, Name: "Кредит", Description: "Пополнение бюджета", ImageURL: "/images/scenario/credit.jpg", IsPositive: true}
	other := api.Event{ID: 7, Name: "Штраф", Description: "Расходы"}
	predictions := []api.Prediction{{Event: zero, Probability: 0.6, Suggested: true}, {Event: other, Probability: 0.4}}
	calculations := []RoundCalculation{
		{TeamID: first.ID, AutoDecision: &DecisionRecord{}, Predictions: predictions, State: api.TeamGameState{Budget: api.Budget{Money: 8}}},
		{TeamID: second.ID, Predictions: predictions, State: api.TeamGameState{Attributes: []api.Attribute{{Value: math.NaN()}}}},
	}
	if err := s.CloseRound(ctx, game.ID, 1, calculations); err == nil {
		t.Fatal("round with invalid second snapshot closed")
	}
	journal = activeJournal(t, s)
	round := journal.Teams[0].Rounds[0]
	if journal.Game.Phase != api.PhaseRoundOpen || round.Result != nil || !reflect.DeepEqual(round.Decision, &saved) {
		t.Fatalf("failed close changed the game or first team's data: %+v, %+v", journal.Game, round)
	}

	calculations[0].AutoDecision = nil
	calculations[1].State = api.TeamGameState{Budget: api.Budget{Money: 1}}
	requireOK(t, s.CloseRound(ctx, game.ID, 1, calculations))
	requireOK(t, s.Close())
	s = openTestStore(t, path)
	journal = activeJournal(t, s)
	round = journal.Teams[0].Rounds[0]
	if journal.Game.Phase != api.PhaseRoundClosed || !reflect.DeepEqual(round.Result.Predictions, predictions) ||
		round.Result.FiredEventIDs != nil || round.Result.StateAfterEvents != nil {
		t.Fatalf("reopened closed round = %+v", round.Result)
	}
	checkFiredEvents(t, s, first.ID, sql.NullString{})
	detail := gameDetail(t, s, game.ID)
	pending := detail.Teams[0].Rounds[0]
	if detail.Game.Rounds != 0 || pending.Events == nil || len(pending.Events) != 0 ||
		pending.State.Budget.Money != 8 || len(pending.State.History) != 0 {
		t.Fatalf("pending history round = %+v, game = %+v", pending, detail.Game)
	}
	skipped := detail.Teams[1].Rounds[0]
	if skipped.Decision != nil || skipped.Cost != nil {
		t.Fatalf("skipped round has a decision or cost: %+v", skipped)
	}

	selections := []RoundEventSelection{
		{TeamID: first.ID, EventIDs: []int{7, 0}, State: api.TeamGameState{Budget: api.Budget{Money: 19}}},
		{TeamID: second.ID, EventIDs: []int{100}},
	}
	if err := s.ApplyEvents(ctx, game.ID, 1, selections, false); err == nil {
		t.Fatal("selection included an event absent from predictions")
	}
	checkFiredEvents(t, s, first.ID, sql.NullString{})
	checkFiredEvents(t, s, second.ID, sql.NullString{})
	if got := activeJournal(t, s).Game; got.Phase != api.PhaseRoundClosed || got.Round != 1 {
		t.Fatalf("failed selection changed the phase or round: %+v", got)
	}

	selections[1].EventIDs = nil
	requireOK(t, s.ApplyEvents(ctx, game.ID, 1, selections, false))
	checkFiredEvents(t, s, first.ID, sql.NullString{String: "[7,0]", Valid: true})
	checkFiredEvents(t, s, second.ID, sql.NullString{String: "[]", Valid: true})
	subsequent := []RoundCalculation{
		{TeamID: second.ID, AutoDecision: &DecisionRecord{Cost: api.Cost{Money: 6}}},
		{TeamID: first.ID, AutoDecision: &DecisionRecord{Cost: api.Cost{Money: 2}}},
	}
	requireOK(t, s.CloseRound(ctx, game.ID, 2, subsequent))
	requireOK(t, s.ApplyEvents(ctx, game.ID, 2, []RoundEventSelection{{TeamID: first.ID}, {TeamID: second.ID}}, false))
	requireOK(t, s.SaveDecision(ctx, game.ID, first.ID, 3, saved))
	requireOK(t, s.FinishGame(ctx, game.ID, 3))
	requireOK(t, s.Close())

	s = openTestStore(t, path)
	journal = activeJournal(t, s)
	rounds = journal.Teams[0].Rounds
	if journal.Game.Phase != api.PhaseFinished || journal.Game.Round != 2 ||
		len(rounds) != 2 || rounds[0].Round != 1 || rounds[1].Round != 2 ||
		!slices.Equal(rounds[0].Result.FiredEventIDs, []int{7, 0}) ||
		!rounds[1].Decision.Auto || rounds[1].Decision.Cost.Money != 2 {
		t.Fatalf("unexpected final journal: %+v, %+v", journal.Game, rounds)
	}
	confirmedEmpty := journal.Teams[1].Rounds[0].Result.FiredEventIDs
	if confirmedEmpty == nil || len(confirmedEmpty) != 0 {
		t.Fatalf("confirmed empty selection = %#v, want a non-nil empty slice", confirmedEmpty)
	}
	detail = gameDetail(t, s, game.ID)
	applied := detail.Teams[0].Rounds[0]
	wantEvents := []api.Event{other, zero}
	if !reflect.DeepEqual(applied.Events, wantEvents) || applied.State.Budget.Money != 19 ||
		!reflect.DeepEqual(applied.State.History, []api.RoundEvents{{Round: 1, Events: wantEvents}}) {
		t.Fatalf("history did not preserve full ordered events and final state: %+v", applied)
	}
	history := detail.Teams[1].Rounds[1].State.History
	if !reflect.DeepEqual(history, []api.RoundEvents{{Round: 1, Events: []api.Event{}}, {Round: 2, Events: []api.Event{}}}) {
		t.Fatalf("empty selections missing from history: %#v", history)
	}
}

func checkFiredEvents(t *testing.T, s *Store, teamID int64, want sql.NullString) {
	t.Helper()
	var got sql.NullString
	requireOK(t, s.db.QueryRowContext(t.Context(), "SELECT fired_events FROM round_results WHERE team_id = ? AND round = 1", teamID).Scan(&got))
	if got != want {
		t.Fatalf("team %d fired_events = %+v, want %+v", teamID, got, want)
	}
}

func TestSetGradesReplacesAtomically(t *testing.T) {
	ctx := t.Context()
	s := openTestStore(t, filepath.Join(t.TempDir(), "game.db"))
	game := createGame(t, s)
	team := registerTeam(t, s, game.ID, "Команда", "token")
	previous := []api.Grade{{Skill: api.SkillRisks, Score: 8, Comment: "Хорошая оценка рисков"}}
	if err := s.SetGrades(ctx, game.ID, team.ID, previous); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("setting grades during an active lobby: got %v, want sql.ErrNoRows", err)
	}
	requireOK(t, s.StartGame(ctx, game.ID, "scenario", []byte("{}")))
	requireOK(t, s.CloseRound(ctx, game.ID, 1, []RoundCalculation{{TeamID: team.ID}}))
	requireOK(t, s.ApplyEvents(ctx, game.ID, 1, []RoundEventSelection{{TeamID: team.ID}}, true))
	if got := activeJournal(t, s).Game; got.Phase != api.PhaseFinished || got.Round != 1 {
		t.Fatalf("finishing through events left the game in %+v", got)
	}
	requireOK(t, s.SetGrades(ctx, game.ID, team.ID, previous))
	createGame(t, s)
	replacement := []api.Grade{{Skill: api.SkillRevision, Score: 10}}
	if err := s.SetGrades(ctx, game.ID, team.ID, append(slices.Clone(replacement), replacement...)); err == nil {
		t.Fatal("duplicate grade was accepted")
	}
	if got := gameDetail(t, s, game.ID).Teams[0].Grades; !reflect.DeepEqual(got, previous) {
		t.Fatalf("failed replacement changed grades to %+v", got)
	}
	requireOK(t, s.SetGrades(ctx, game.ID, team.ID, replacement))
	if got := gameDetail(t, s, game.ID).Teams[0].Grades; !reflect.DeepEqual(got, replacement) {
		t.Fatalf("grades = %+v, want %+v", got, replacement)
	}
	requireOK(t, s.SetGrades(ctx, game.ID, team.ID, nil))
	if got := gameDetail(t, s, game.ID).Teams[0].Grades; got == nil || len(got) != 0 {
		t.Fatalf("cleared grades = %#v, want a non-nil empty slice", got)
	}
}
