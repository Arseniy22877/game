package game

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/Arseniy22877/game/internal/enginebridge"
	"github.com/Arseniy22877/game/internal/store"
)

type recorder struct {
	Notifier
	messages []api.WSMessage
}

func (r *recorder) ToAll(m api.WSMessage) { r.messages = append(r.messages, m) }
func (r *recorder) Online(int64) bool     { return false }
func (r *recorder) DropAllTeams()         {}
func TestRecoveryAndRollback(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "game.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	catalog, err := enginebridge.LoadCatalog("../enginebridge/testdata")
	if err != nil {
		t.Fatal(err)
	}
	r := &recorder{}
	restore := func() *Manager {
		t.Helper()
		m, err := NewManager(ctx, db, catalog, r)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	m := restore()
	if m.GameInfo().Phase != api.PhaseNone {
		t.Fatal(m.GameInfo())
	}
	g, err := m.CreateGame(ctx, false)
	if err != nil || !reflect.DeepEqual(m.AdminState(), restore().AdminState()) {
		t.Fatal(g, err)
	}
	_, err = db.RegisterTeam(ctx, g.GameID, "A", []string{"B"}, "token")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../enginebridge/testdata/full.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.StartGame(ctx, g.GameID, "small", raw); err != nil {
		t.Fatal(err)
	}
	m = restore()
	before := m.AdminState()
	if v, err := m.TeamState("token"); err != nil || v.State == nil {
		t.Fatal(v, err)
	}
	var denied *Error
	if _, err = m.CreateGame(ctx, false); !errors.As(err, &denied) || denied.Code != api.CodeGameInProgress {
		t.Fatal(err)
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if _, err = sqlDB.Exec("CREATE TRIGGER fail_game BEFORE INSERT ON games BEGIN SELECT RAISE(ABORT,'injected'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err = m.CreateGame(ctx, true); err == nil || !reflect.DeepEqual(before, m.AdminState()) || !reflect.DeepEqual(before, restore().AdminState()) || len(r.messages) != 1 {
		t.Fatal("rollback", err)
	}
	if _, err = sqlDB.Exec("DROP TRIGGER fail_game"); err != nil {
		t.Fatal(err)
	}
	next, err := m.CreateGame(ctx, true)
	if err != nil || next.GameID == g.GameID || !reflect.DeepEqual(m.AdminState(), restore().AdminState()) {
		t.Fatal(next, err)
	}
	var active int
	if err = sqlDB.QueryRow("SELECT count(*) FROM games WHERE active=1").Scan(&active); err != nil || active != 1 {
		t.Fatal(active, err)
	}
	db.Close()
	if _, err = NewManager(ctx, db, catalog, r); err == nil {
		t.Fatal("storage failure treated as empty database")
	}
}
