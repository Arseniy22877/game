// Package api содержит DTO контракта фронт↔бэк: только типы и константы.
// Описание контракта: docs/api.md. Меняется только по согласованию с фронтом.
//
// Пакет ничего не импортирует из internal/, чтобы его могли использовать
// enginebridge, game, store и httpapi без циклов. Хендлеры лежат в internal/httpapi.
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
	CodeTeamsNotReady     = "teams_not_ready"
	CodeGameInProgress    = "game_in_progress"
	CodeTeamBankrupt      = "team_bankrupt"
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
	Bankrupt  bool     `json:"bankrupt"`
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
	Suggested   bool    `json:"suggested"` // выпало в розыгрыше движка, подсказка преподу
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
	Token       string         `json:"token"`
	State       *TeamGameState `json:"state"`
	Decision    *Decision      `json:"decision"`
	Cost        *Cost          `json:"cost"`
	Predictions []Prediction   `json:"predictions"`
}

type AdminState struct {
	Game  GameInfo    `json:"game"`
	Teams []AdminTeam `json:"teams"`
}

type CreateGameRequest struct {
	Confirm bool `json:"confirm"`
}

type StartGameRequest struct {
	ScenarioID string `json:"scenarioId"`
}

type CloseRoundRequest struct {
	Round int  `json:"round"`
	Force bool `json:"force"`
}

type EventSelection struct {
	TeamID   int64 `json:"teamId"`
	EventIDs []int `json:"eventIds"`
}

type FireEventsRequest struct {
	Round      int              `json:"round"`
	Finish     bool             `json:"finish"`
	Selections []EventSelection `json:"selections"`
}

// Навыки, по которым препод оценивает команду за игру (слайд «Навыки для оценки» в ТЗ).
const (
	SkillProblemStatement = "problem_statement" // Постановка задачи
	SkillFactChecking     = "fact_checking"     // Проверка информации
	SkillConnections      = "connections"       // Поиск связей
	SkillAlternatives     = "alternatives"      // Рассмотрение альтернатив и возражений
	SkillRisks            = "risks"             // Оценка рисков
	SkillRevision         = "revision"          // Пересмотр и объяснение
)

// Skills — все навыки в порядке слайда.
var Skills = []string{
	SkillProblemStatement, SkillFactChecking, SkillConnections,
	SkillAlternatives, SkillRisks, SkillRevision,
}

type Grade struct {
	Skill   string `json:"skill"`
	Score   int    `json:"score"`
	Comment string `json:"comment"`
}

// Grades — тело запроса и ответа PUT /api/admin/games/{gameId}/teams/{teamId}/grades.
type Grades struct {
	Grades []Grade `json:"grades"`
}

type GameHistoryItem struct {
	GameID     int64     `json:"gameId"`
	ScenarioID string    `json:"scenarioId"`
	Phase      Phase     `json:"phase"`
	Rounds     int       `json:"rounds"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"createdAt"`
}

type GamesResponse struct {
	Games []GameHistoryItem `json:"games"`
}

// GameDetail — ответ GET /api/admin/games/{gameId}.
type GameDetail struct {
	Game  GameHistoryItem  `json:"game"`
	Teams []GameDetailTeam `json:"teams"`
}

type GameDetailTeam struct {
	ID      int64             `json:"id"`
	Name    string            `json:"name"`
	Members []string          `json:"members"`
	Rounds  []GameDetailRound `json:"rounds"`
	Grades  []Grade           `json:"grades"`
}

type GameDetailRound struct {
	Round       int           `json:"round"`
	Decision    *Decision     `json:"decision"` // nil, если команда пропустила раунд (bankrupt)
	Cost        *Cost         `json:"cost"`
	Auto        bool          `json:"auto"` // решение по умолчанию при force
	Predictions []Prediction  `json:"predictions"`
	Events      []Event       `json:"events"`
	State       TeamGameState `json:"state"`
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
