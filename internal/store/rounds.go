package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Arseniy22877/game/internal/api"
)

type DecisionRecord struct {
	Decision api.Decision
	Cost     api.Cost
	Auto     bool
}

type RoundCalculation struct {
	TeamID       int64
	AutoDecision *DecisionRecord // nil for a submitted decision or a skipped round
	Predictions  []api.Prediction
	State        api.TeamGameState
}

type RoundEventSelection struct {
	TeamID   int64
	EventIDs []int
	State    api.TeamGameState
}

func (s *Store) SaveDecision(ctx context.Context, gameID, teamID int64, round int, record DecisionRecord) error {
	return saveDecision(ctx, s.db, gameID, teamID, round, record, now())
}

func saveDecision(ctx context.Context, db executor, gameID, teamID int64, round int, record DecisionRecord, timestamp string) error {
	record.Decision.Attributes = array(record.Decision.Attributes)
	record.Decision.Strategies = array(record.Decision.Strategies)
	record.Decision.Hints = array(record.Decision.Hints)
	data, err := encode(record.Decision)
	if err != nil {
		return err
	}
	return execOne(ctx, db, `
		INSERT INTO decisions (team_id, round, decision, cost_money, cost_qp, auto, updated_at)
		SELECT t.id, g.round, ?, ?, ?, ?, ? FROM teams t JOIN games g ON t.game_id = g.id
		WHERE t.id = ? AND g.id = ? AND g.active = 1 AND g.phase = ? AND g.round = ?
		ON CONFLICT (team_id, round) DO UPDATE SET
			decision = excluded.decision, cost_money = excluded.cost_money,
			cost_qp = excluded.cost_qp, auto = excluded.auto, updated_at = excluded.updated_at`,
		data, record.Cost.Money, record.Cost.QP, record.Auto, timestamp,
		teamID, gameID, api.PhaseRoundOpen, round)
}

// CloseRound saves the calculations and any default decisions for all teams.
func (s *Store) CloseRound(ctx context.Context, gameID int64, round int, calculations []RoundCalculation) error {
	return transaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := checkTeamCount(ctx, tx, gameID, len(calculations)); err != nil {
			return err
		}
		timestamp := now()
		for _, calc := range calculations {
			if calc.AutoDecision != nil {
				record := *calc.AutoDecision
				record.Auto = true
				if err := saveDecision(ctx, tx, gameID, calc.TeamID, round, record, timestamp); err != nil {
					return err
				}
			}
			predictions, err := encode(array(calc.Predictions))
			if err != nil {
				return err
			}
			state, err := encodeState(calc.State)
			if err != nil {
				return err
			}
			err = execOne(ctx, tx, `
				INSERT INTO round_results (team_id, round, predictions, state_after_calc, closed_at)
				SELECT id, ?, ?, ?, ? FROM teams WHERE id = ? AND game_id = ?`,
				round, predictions, state, timestamp, calc.TeamID, gameID)
			if err != nil {
				return fmt.Errorf("save calculation for team %d: %w", calc.TeamID, err)
			}
		}
		return execOne(ctx, tx, `
			UPDATE games SET phase = ? WHERE id = ? AND active = 1 AND phase = ? AND round = ?`,
			api.PhaseRoundClosed, gameID, api.PhaseRoundOpen, round)
	})
}

// ApplyEvents saves the selections and snapshots for all teams, then advances the game.
func (s *Store) ApplyEvents(ctx context.Context, gameID int64, round int, selections []RoundEventSelection, finish bool) error {
	return transaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := checkTeamCount(ctx, tx, gameID, len(selections)); err != nil {
			return err
		}
		timestamp := now()
		for _, selection := range selections {
			if err := saveEvents(ctx, tx, gameID, round, selection, timestamp); err != nil {
				return fmt.Errorf("save events for team %d: %w", selection.TeamID, err)
			}
		}
		phase, nextRound := api.PhaseRoundOpen, round+1
		var finishedAt any
		if finish {
			phase, nextRound, finishedAt = api.PhaseFinished, round, timestamp
		}
		return execOne(ctx, tx, `
			UPDATE games SET phase = ?, round = ?, finished_at = ?
			WHERE id = ? AND active = 1 AND phase = ? AND round = ?`,
			phase, nextRound, finishedAt, gameID, api.PhaseRoundClosed, round)
	})
}

func saveEvents(ctx context.Context, tx *sql.Tx, gameID int64, round int, selection RoundEventSelection, timestamp string) error {
	ids := array(selection.EventIDs)
	events, err := encode(ids)
	if err != nil {
		return err
	}
	state, err := encodeState(selection.State)
	if err != nil {
		return err
	}
	var predictions string
	err = tx.QueryRowContext(ctx, `
		UPDATE round_results SET fired_events = ?, state_after_events = ?, events_at = ?
		WHERE team_id = (SELECT id FROM teams WHERE id = ? AND game_id = ?)
			AND round = ? AND fired_events IS NULL
		RETURNING predictions`, events, state, timestamp, selection.TeamID, gameID, round).Scan(&predictions)
	if err != nil {
		return err
	}
	result := RoundResult{FiredEventIDs: ids}
	if err := json.Unmarshal([]byte(predictions), &result.Predictions); err != nil {
		return err
	}
	_, err = result.Events()
	return err
}

func checkTeamCount(ctx context.Context, tx *sql.Tx, gameID int64, count int) error {
	var teams int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM teams WHERE game_id = ?", gameID).Scan(&teams); err != nil {
		return err
	}
	if teams != count {
		return fmt.Errorf("game %d has %d teams, got %d round entries", gameID, teams, count)
	}
	return nil
}

func encodeState(state api.TeamGameState) (string, error) {
	state.Attributes = array(state.Attributes)
	state.Configurable = array(state.Configurable)
	state.Strategies = array(state.Strategies)
	state.Hints = array(state.Hints)
	history := make([]api.RoundEvents, len(state.History))
	for i, entry := range state.History {
		entry.Events = array(entry.Events)
		history[i] = entry
	}
	state.History = history
	return encode(state)
}
