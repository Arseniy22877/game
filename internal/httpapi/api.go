package httpapi

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/Arseniy22877/game/internal/game"
)

type serverAPI struct {
	logger     *slog.Logger
	manager    *game.Manager
	adminToken string
}

func teamToken(r *http.Request) string {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return token
}

func (s *serverAPI) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := teamToken(r)
		if token == "" || subtle.ConstantTimeCompare([]byte(s.adminToken), []byte(token)) != 1 {
			writeError(w, http.StatusUnauthorized, api.CodeUnauthorized, "Нужен токен преподавателя")
			return
		}
		next(w, r)
	}
}

func (s *serverAPI) team(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.manager.TeamID(teamToken(r)); err != nil {
			s.respond(w, nil, err)
			return
		}
		next(w, r)
	}
}

func badRequest(message string) error {
	return &game.Error{Status: http.StatusBadRequest, Code: api.CodeBadRequest, Message: message}
}

func pathID(r *http.Request, key string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	if err != nil || id < 1 {
		return 0, badRequest("Некорректный идентификатор")
	}
	return id, nil
}

func readJSON(r *http.Request, target any, allowEmpty bool) error {
	body, err := io.ReadAll(r.Body) // middleware limits all request bodies to 64 KiB.
	if err != nil {
		return badRequest("Не удалось прочитать тело запроса или превышен лимит 64 КБ")
	}
	body = bytes.TrimSpace(body)
	if allowEmpty && len(body) == 0 {
		return nil
	}
	if len(body) == 0 || body[0] != '{' || json.Unmarshal(body, target) != nil {
		return badRequest("Ожидался объект JSON с полями указанного типа")
	}
	return nil
}

func (s *serverAPI) respond(w http.ResponseWriter, value any, err error) {
	if err == nil {
		writeJSON(w, http.StatusOK, value)
		return
	}
	var e *game.Error
	if errors.As(err, &e) {
		writeError(w, e.Status, e.Code, e.Message)
		return
	}
	s.logger.Error("game request failed", "error", err)
	writeError(w, http.StatusInternalServerError, api.CodeInternal, "Внутренняя ошибка сервера")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
