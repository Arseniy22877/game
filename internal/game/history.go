package game

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/Arseniy22877/game/internal/store"
)

func (m *Manager) Games(ctx context.Context) (api.GamesResponse, error) {
	games, err := m.store.Games(ctx)
	return api.GamesResponse{Games: games}, err
}

func (m *Manager) GameDetail(ctx context.Context, gameID int64) (api.GameDetail, error) {
	detail, err := m.store.GameDetail(ctx, gameID)
	if errors.Is(err, sql.ErrNoRows) {
		return api.GameDetail{}, &Error{http.StatusNotFound, api.CodeNotFound, "Игра не найдена"}
	}
	return detail, err
}

func (m *Manager) SetGrades(ctx context.Context, gameID, teamID int64, grades []api.Grade) (api.Grades, error) {
	normalized := make([]api.Grade, 0, len(grades))
	seen := make(map[string]bool, len(api.Skills))
	for _, grade := range grades {
		grade.Skill = strings.TrimSpace(grade.Skill)
		grade.Comment = strings.TrimSpace(grade.Comment)
		if !slices.Contains(api.Skills, grade.Skill) || seen[grade.Skill] {
			return api.Grades{}, &Error{http.StatusBadRequest, api.CodeBadRequest, "Укажите каждый известный навык не более одного раза"}
		}
		if grade.Score < 0 || grade.Score > 10 {
			return api.Grades{}, &Error{http.StatusBadRequest, api.CodeBadRequest, "Оценка должна быть от 0 до 10"}
		}
		if utf8.RuneCountInString(grade.Comment) > 2000 {
			return api.Grades{}, &Error{http.StatusBadRequest, api.CodeBadRequest, "Комментарий не должен превышать 2000 символов"}
		}
		seen[grade.Skill] = true
		normalized = append(normalized, grade)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.store.SetGrades(ctx, gameID, teamID, normalized)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return api.Grades{}, &Error{http.StatusNotFound, api.CodeNotFound, "Команда в указанной игре не найдена"}
	case errors.Is(err, store.ErrGameNotGradable):
		return api.Grades{}, &Error{http.StatusConflict, api.CodeWrongPhase, "Оценки доступны после завершения или архивации игры"}
	case err != nil:
		return api.Grades{}, err
	default:
		return api.Grades{Grades: normalized}, nil
	}
}
