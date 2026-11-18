package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Arseniy22877/game/internal/api"
)

type Journal struct {
	Game  Game
	Teams []JournalTeam
}

type JournalTeam struct {
	Team
	Rounds []JournalRound // ascending, including a submitted decision in the open round
}

type JournalRound struct {
	Round    int
	Decision *DecisionRecord // nil when the team skipped the round
	Result   *RoundResult    // nil until the round closes
}

type RoundResult struct {
	Predictions      []api.Prediction
	StateAfterCalc   api.TeamGameState
	FiredEventIDs    []int // nil while awaiting selection, empty after a confirmed empty selection
	StateAfterEvents *api.TeamGameState
}

// Events resolves the applied IDs against the saved predictions, in application order.
func (r RoundResult) Events() ([]api.Event, error) {
	if r.FiredEventIDs == nil {
		return nil, nil
	}
	byID := make(map[int]api.Event, len(r.Predictions))
	for _, p := range r.Predictions {
		byID[p.Event.ID] = p.Event
	}
	events := make([]api.Event, 0, len(r.FiredEventIDs))
	for _, id := range r.FiredEventIDs {
		event, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("event %d is missing from predictions or selected twice", id)
		}
		events = append(events, event)
		delete(byID, id)
	}
	return events, nil
}

const selectGame = "SELECT id, phase, round, active, scenario_id, scenario_json, created_at FROM games"

type scanner interface {
	Scan(...any) error
}

func scanGame(row scanner) (Game, error) {
	var game Game
	var createdAt string
	err := row.Scan(&game.ID, &game.Phase, &game.Round, &game.Active,
		&game.ScenarioID, &game.ScenarioJSON, &createdAt)
	if err != nil {
		return Game{}, err
	}
	game.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	return game, err
}

// ActiveJournal reads a consistent snapshot of the active game. It returns
// sql.ErrNoRows if there is no active game.
func (s *Store) ActiveJournal(ctx context.Context) (Journal, error) {
	var journal Journal
	err := transaction(ctx, s.db, func(tx *sql.Tx) error {
		var err error
		journal, err = loadJournal(ctx, tx, selectGame+" WHERE active = 1")
		return err
	})
	if err != nil {
		return Journal{}, err
	}
	return journal, nil
}

func loadJournal(ctx context.Context, tx *sql.Tx, query string, args ...any) (Journal, error) {
	game, err := scanGame(tx.QueryRowContext(ctx, query, args...))
	if err != nil {
		return Journal{}, err
	}
	journal := Journal{Game: game, Teams: []JournalTeam{}}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, name, members, token, ready FROM teams WHERE game_id = ? ORDER BY id`, game.ID)
	if err != nil {
		return Journal{}, err
	}
	defer rows.Close()
	for rows.Next() {
		team := JournalTeam{Rounds: []JournalRound{}}
		var members string
		if err := rows.Scan(&team.ID, &team.Name, &members, &team.Token, &team.Ready); err != nil {
			return Journal{}, err
		}
		if err := json.Unmarshal([]byte(members), &team.Members); err != nil {
			return Journal{}, fmt.Errorf("read members for team %d: %w", team.ID, err)
		}
		journal.Teams = append(journal.Teams, team)
	}
	if err := rows.Err(); err != nil {
		return Journal{}, err
	}
	for i := range journal.Teams {
		team := &journal.Teams[i]
		if err := loadRounds(ctx, tx, team, game.Round); err != nil {
			return Journal{}, fmt.Errorf("read rounds for team %d: %w", team.ID, err)
		}
	}
	return journal, nil
}

func loadRounds(ctx context.Context, tx *sql.Tx, team *JournalTeam, lastRound int) error {
	rows, err := tx.QueryContext(ctx, `
		WITH rounds AS (
			SELECT round FROM decisions WHERE team_id = ?1
			UNION SELECT round FROM round_results WHERE team_id = ?1
		)
		SELECT r.round, d.decision, d.cost_money, d.cost_qp, d.auto,
			c.predictions, c.state_after_calc, c.fired_events, c.state_after_events
		FROM rounds r
		LEFT JOIN decisions d ON d.team_id = ?1 AND d.round = r.round
		LEFT JOIN round_results c ON c.team_id = ?1 AND c.round = r.round
		WHERE r.round <= ?2 ORDER BY r.round`, team.ID, lastRound)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		round, err := scanRound(rows)
		if err != nil {
			return fmt.Errorf("round %d: %w", round.Round, err)
		}
		team.Rounds = append(team.Rounds, round)
	}
	return rows.Err()
}

func scanRound(row scanner) (JournalRound, error) {
	var round JournalRound
	var decision, predictions, calculated, fired, applied sql.NullString
	var money, qp sql.Null[int]
	var auto sql.NullBool
	err := row.Scan(&round.Round, &decision, &money, &qp, &auto, &predictions, &calculated, &fired, &applied)
	if err != nil {
		return round, err
	}
	if decision.Valid {
		record := &DecisionRecord{Cost: api.Cost{Money: money.V, QP: qp.V}, Auto: auto.Bool}
		if err := json.Unmarshal([]byte(decision.String), &record.Decision); err != nil {
			return round, fmt.Errorf("decision: %w", err)
		}
		round.Decision = record
	}
	if !predictions.Valid {
		return round, nil
	}
	result := &RoundResult{}
	if err := json.Unmarshal([]byte(predictions.String), &result.Predictions); err != nil {
		return round, fmt.Errorf("predictions: %w", err)
	}
	if err := json.Unmarshal([]byte(calculated.String), &result.StateAfterCalc); err != nil {
		return round, fmt.Errorf("state after calculation: %w", err)
	}
	if fired.Valid {
		if err := json.Unmarshal([]byte(fired.String), &result.FiredEventIDs); err != nil {
			return round, fmt.Errorf("fired events: %w", err)
		}
		if result.FiredEventIDs == nil {
			return round, fmt.Errorf("fired events must be a JSON array")
		}
		result.StateAfterEvents = new(api.TeamGameState)
		if err := json.Unmarshal([]byte(applied.String), result.StateAfterEvents); err != nil {
			return round, fmt.Errorf("state after events: %w", err)
		}
	}
	round.Result = result
	return round, nil
}
