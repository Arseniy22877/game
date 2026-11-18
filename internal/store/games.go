package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Arseniy22877/game/internal/api"
)

type Game struct {
	ID           int64
	Phase        api.Phase
	Round        int
	Active       bool
	ScenarioID   string
	ScenarioJSON []byte
	CreatedAt    time.Time
}

type Team struct {
	ID      int64
	Name    string
	Members []string
	Token   string
	Ready   bool
}

// CreateGame archives the current game and creates its replacement atomically.
// The caller checks whether the replacement needs confirmation.
func (s *Store) CreateGame(ctx context.Context) (Game, error) {
	game := Game{
		Phase:     api.PhaseLobby,
		Active:    true,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	err := transaction(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE games SET active = 0 WHERE active = 1"); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, "INSERT INTO games (phase, created_at) VALUES (?, ?) RETURNING id",
			game.Phase, game.CreatedAt.Format(time.RFC3339)).Scan(&game.ID)
	})
	if err != nil {
		return Game{}, err
	}
	return game, nil
}

func (s *Store) RegisterTeam(ctx context.Context, gameID int64, name string, members []string, token string) (Team, error) {
	name = strings.TrimSpace(name)
	team := Team{Name: name, Members: array(members), Token: token, Ready: true}
	membersJSON, err := encode(team.Members)
	if err != nil {
		return Team{}, err
	}
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO teams (game_id, name, name_key, members, token, created_at)
		SELECT id, ?, ?, ?, ?, ? FROM games WHERE id = ? AND active = 1 AND phase = ?
		RETURNING id`,
		team.Name, strings.ToLower(team.Name), membersJSON, team.Token, now(), gameID, api.PhaseLobby).Scan(&team.ID)
	if err != nil {
		return Team{}, err
	}
	return team, nil
}

func (s *Store) SetReady(ctx context.Context, gameID, teamID int64, ready bool) error {
	return execOne(ctx, s.db, `
		UPDATE teams SET ready = ? WHERE id = ? AND game_id = (
			SELECT id FROM games WHERE id = ? AND active = 1 AND phase = ?
		)`, ready, teamID, gameID, api.PhaseLobby)
}

func (s *Store) DeleteTeam(ctx context.Context, gameID, teamID int64) error {
	return execOne(ctx, s.db, `
		DELETE FROM teams WHERE id = ? AND game_id = (
			SELECT id FROM games WHERE id = ? AND active = 1 AND phase = ?
		)`, teamID, gameID, api.PhaseLobby)
}

func (s *Store) StartGame(ctx context.Context, gameID int64, scenarioID string, scenarioJSON []byte) error {
	return execOne(ctx, s.db, `
		UPDATE games SET phase = ?, round = 1, scenario_id = ?, scenario_json = ?, started_at = ?
		WHERE id = ? AND active = 1 AND phase = ?`,
		api.PhaseRoundOpen, scenarioID, string(scenarioJSON), now(), gameID, api.PhaseLobby)
}

// FinishGame ends an open game at the last fully played round.
func (s *Store) FinishGame(ctx context.Context, gameID int64, round int) error {
	return execOne(ctx, s.db, `
		UPDATE games SET phase = ?, round = round - 1, finished_at = ?
		WHERE id = ? AND active = 1 AND phase = ? AND round = ?`,
		api.PhaseFinished, now(), gameID, api.PhaseRoundOpen, round)
}

// ErrGameNotGradable indicates that the team belongs to an active, unfinished game.
var ErrGameNotGradable = errors.New("active game has not finished")

// SetGrades replaces the complete set of grades for a team.
func (s *Store) SetGrades(ctx context.Context, gameID, teamID int64, grades []api.Grade) error {
	return transaction(ctx, s.db, func(tx *sql.Tx) error {
		var active bool
		var phase api.Phase
		err := tx.QueryRowContext(ctx, `
			SELECT g.active, g.phase FROM teams t JOIN games g ON g.id = t.game_id
			WHERE t.id = ? AND g.id = ?`, teamID, gameID).Scan(&active, &phase)
		if err != nil {
			return err
		}
		if active && phase != api.PhaseFinished {
			return ErrGameNotGradable
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM grades WHERE team_id = ?", teamID); err != nil {
			return err
		}
		timestamp := now()
		for _, grade := range grades {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO grades (team_id, skill, score, comment, updated_at) VALUES (?, ?, ?, ?, ?)`,
				teamID, grade.Skill, grade.Score, grade.Comment, timestamp); err != nil {
				return err
			}
		}
		return nil
	})
}
