package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Arseniy22877/game/internal/api"
)

func (g Game) historyItem() api.GameHistoryItem {
	rounds := g.Round
	if g.Phase == api.PhaseRoundOpen || g.Phase == api.PhaseRoundClosed {
		rounds--
	}
	return api.GameHistoryItem{
		GameID: g.ID, Phase: g.Phase, ScenarioID: g.ScenarioID,
		Rounds: rounds, Active: g.Active, CreatedAt: g.CreatedAt,
	}
}

func (s *Store) Games(ctx context.Context) ([]api.GameHistoryItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, phase, round, active, scenario_id, NULL, created_at FROM games ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	games := []api.GameHistoryItem{}
	for rows.Next() {
		game, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		games = append(games, game.historyItem())
	}
	return games, rows.Err()
}

func (s *Store) GameDetail(ctx context.Context, gameID int64) (api.GameDetail, error) {
	var detail api.GameDetail
	err := transaction(ctx, s.db, func(tx *sql.Tx) error {
		journal, err := loadJournal(ctx, tx, selectGame+" WHERE id = ?", gameID)
		if err != nil {
			return err
		}
		detail = api.GameDetail{Game: journal.Game.historyItem(), Teams: []api.GameDetailTeam{}}
		for _, team := range journal.Teams {
			rounds, err := detailRounds(team.Rounds)
			if err != nil {
				return fmt.Errorf("read history for team %d: %w", team.ID, err)
			}
			grades, err := loadGrades(ctx, tx, team.ID)
			if err != nil {
				return err
			}
			detail.Teams = append(detail.Teams, api.GameDetailTeam{
				ID: team.ID, Name: team.Name, Members: team.Members, Rounds: rounds, Grades: grades,
			})
		}
		return nil
	})
	if err != nil {
		return api.GameDetail{}, err
	}
	return detail, nil
}

func detailRounds(journal []JournalRound) ([]api.GameDetailRound, error) {
	rounds := []api.GameDetailRound{}
	history := []api.RoundEvents{}
	for _, entry := range journal {
		if entry.Result == nil {
			continue
		}
		result := entry.Result
		events, err := result.Events()
		if err != nil {
			return nil, fmt.Errorf("round %d: %w", entry.Round, err)
		}
		state := result.StateAfterCalc
		if events != nil {
			state = *result.StateAfterEvents
			history = append(history, api.RoundEvents{Round: entry.Round, Events: events})
		}
		state.History = history[:len(history):len(history)]
		round := api.GameDetailRound{
			Round: entry.Round, Predictions: result.Predictions, Events: array(events), State: state,
		}
		if entry.Decision != nil {
			round.Decision = &entry.Decision.Decision
			round.Cost = &entry.Decision.Cost
			round.Auto = entry.Decision.Auto
		}
		rounds = append(rounds, round)
	}
	return rounds, nil
}

func loadGrades(ctx context.Context, tx *sql.Tx, teamID int64) ([]api.Grade, error) {
	rows, err := tx.QueryContext(ctx, "SELECT skill, score, comment FROM grades WHERE team_id = ? ORDER BY skill", teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grades := []api.Grade{}
	for rows.Next() {
		var grade api.Grade
		if err := rows.Scan(&grade.Skill, &grade.Score, &grade.Comment); err != nil {
			return nil, err
		}
		grades = append(grades, grade)
	}
	return grades, rows.Err()
}
