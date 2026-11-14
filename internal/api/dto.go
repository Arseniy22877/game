// Package api содержит HTTP-хендлеры и DTO контракта фронт↔бэк.
// Описание контракта: docs/api.md. Меняется только по согласованию с фронтом.
package api

import "time"

// Phase — фаза активной игры.
type Phase string

const (
	PhaseNone        Phase = "none"
	PhaseLobby       Phase = "lobby"
	PhaseRoundOpen   Phase = "round_open"
	PhaseRoundClosed Phase = "round_closed"
	PhaseFinished    Phase = "finished"
)

// Коды ошибок из docs/api.md.
const (
	CodeBadRequest        = "bad_request"
	CodeUnauthorized      = "unauthorized"
	CodeNotFound          = "not_found"
	CodeWrongPhase        = "wrong_phase"
	CodeNoGame            = "no_game"
	CodeNameTaken         = "name_taken"
	CodeNotAllSubmitted   = "not_all_submitted"
	CodeNotAllTeams       = "not_all_teams"
	CodeInvalidDecision   = "invalid_decision"
	CodeInsufficientMoney = "insufficient_money"
	CodeInsufficientQP    = "insufficient_qp"
	CodeInvalidEvent      = "invalid_event"
	CodeInternal          = "internal"
)

// ErrorResponse — тело любого ответа 4xx/5xx.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ---------- Общие объекты ----------

type GameInfo struct {
	GameID        int64  `json:"gameId"`
	Phase         Phase  `json:"phase"`
	Round         int    `json:"round"`
	ScenarioID    string `json:"scenarioId"`
	BaseRoundCost int    `json:"baseRoundCost"`
}

type TeamSummary struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Members   []string `json:"members"`
	Ready     bool     `json:"ready"`
	Online    bool     `json:"online"`
	Submitted bool     `json:"submitted"`
}

type Budget struct {
	Money    int `json:"money"`
	MaxMoney int `json:"maxMoney"`
	QP       int `json:"qp"`
}

type Attribute struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Value       float64 `json:"value"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
}

type ConfigurableAttribute struct {
	Attribute
	CostPerUnit float64 `json:"costPerUnit"`
}

type Strategy struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Cost        int    `json:"cost"`
	Active      bool   `json:"active"`
}

type Hint struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Reliability string `json:"reliability"`
	Cost        int    `json:"cost"`
	Used        bool   `json:"used"`
	Secret      string `json:"secret"` // пусто, пока Used == false
}

type Event struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ImageURL    string `json:"imageUrl"`
	IsPositive  bool   `json:"isPositive"`
}

type RoundEvents struct {
	Round  int     `json:"round"`
	Events []Event `json:"events"`
}

// TeamGameState — состояние экземпляра игры одной команды.
type TeamGameState struct {
	Budget       Budget                  `json:"budget"`
	Attributes   []Attribute             `json:"attributes"`
	Configurable []ConfigurableAttribute `json:"configurable"`
	Strategies   []Strategy              `json:"strategies"`
	Hints        []Hint                  `json:"hints"`
	History      []RoundEvents           `json:"history"`
}

type AttributeChoice struct {
	ID    int     `json:"id"`
	Value float64 `json:"value"`
}

// Report — отчёт команды из 4 вопросов (в движке DecisionData).
type Report struct {
	Why         string `json:"why"`         // Почему именно такое решение
	Alternative string `json:"alternative"` // Какое могло быть другое
	Risks       string `json:"risks"`       // Какие риски с ним связаны
	Correction  string `json:"correction"`  // Что изменили бы, если бы это была не игра, а реальная жизнь
}

type Decision struct {
	Attributes []AttributeChoice `json:"attributes"`
	Strategies []int             `json:"strategies"`
	Hints      []int             `json:"hints"`
	Report     Report            `json:"report"`
}

type Cost struct {
	Money int `json:"money"`
	QP    int `json:"qp"`
}

type Prediction struct {
	Event       Event   `json:"event"`
	Probability float64 `json:"probability"`
}

// ---------- Команда ----------

type RegisterTeamRequest struct {
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

type RegisterTeamResponse struct {
	Token string      `json:"token"`
	Team  TeamSummary `json:"team"`
}

type SetReadyRequest struct {
	Ready bool `json:"ready"`
}

type TeamStateResponse struct {
	Game     GameInfo       `json:"game"`
	Team     TeamSummary    `json:"team"`
	Opening  *Event         `json:"opening"`
	State    *TeamGameState `json:"state"`
	Decision *Decision      `json:"decision"`
}

type SubmitDecisionRequest struct {
	Round int `json:"round"`
	Decision
}

type SubmitDecisionResponse struct {
	Cost Cost `json:"cost"`
}

// ---------- Преподаватель ----------

type Scenario struct {
	ID string `json:"id"`
}

type ScenariosResponse struct {
	Scenarios []Scenario `json:"scenarios"`
}

type AdminTeam struct {
	TeamSummary
	State       *TeamGameState `json:"state"`
	Decision    *Decision      `json:"decision"`
	Cost        *Cost          `json:"cost"`
	Predictions []Prediction   `json:"predictions"`
}

type AdminState struct {
	Game  GameInfo    `json:"game"`
	Teams []AdminTeam `json:"teams"`
}

type StartGameRequest struct {
	ScenarioID string `json:"scenarioId"`
}

type CloseRoundRequest struct {
	Force bool `json:"force"`
}

type EventSelection struct {
	TeamID   int64 `json:"teamId"`
	EventIDs []int `json:"eventIds"`
}

type FireEventsRequest struct {
	Selections []EventSelection `json:"selections"`
}

type Grade struct {
	AttributeID int    `json:"attributeId"`
	Score       int    `json:"score"`
	Comment     string `json:"comment"`
}

type GradesRequest struct {
	TeamID int64   `json:"teamId"`
	Grades []Grade `json:"grades"`
}

type GameHistoryItem struct {
	GameID     int64     `json:"gameId"`
	ScenarioID string    `json:"scenarioId"`
	Phase      Phase     `json:"phase"`
	Rounds     int       `json:"rounds"`
	CreatedAt  time.Time `json:"createdAt"`
}

type GamesResponse struct {
	Games []GameHistoryItem `json:"games"`
}

// ---------- WebSocket ----------

// WS-типы сообщений сервер → клиент.
const (
	WSPhaseChanged      = "phase_changed"      // payload: GameInfo
	WSLobbyUpdated      = "lobby_updated"      // payload: LobbyUpdatedPayload
	WSSubmissionUpdated = "submission_updated" // payload: SubmissionUpdatedPayload
	WSTeamOnline        = "team_online"        // payload: TeamOnlinePayload
	WSTeamState         = "team_state"         // payload: RoundEvents
)

type WSMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type LobbyUpdatedPayload struct {
	Teams []TeamSummary `json:"teams"`
}

type SubmissionUpdatedPayload struct {
	TeamID    int64 `json:"teamId"`
	Submitted bool  `json:"submitted"`
}

type TeamOnlinePayload struct {
	TeamID int64 `json:"teamId"`
	Online bool  `json:"online"`
}
