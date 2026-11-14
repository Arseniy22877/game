// Типы контракта фронт ↔ бэк (docs/api.md)

// --- Фазы игры ---
export type Phase = 'none' | 'lobby' | 'round_open' | 'round_closed' | 'finished';

// --- Общие объекты ---

export interface GameInfo {
  gameId: number;
  phase: Phase;
  round: number;
  scenarioId: string;
  baseRoundCost: number;
}

export interface TeamSummary {
  id: number;
  name: string;
  members: string[];
  ready: boolean;
  online: boolean;
  submitted: boolean;
  bankrupt: boolean;
}

export interface Budget {
  money: number;
  maxMoney: number;
  qp: number;
}

export interface Attribute {
  id: number;
  name: string;
  description: string;
  value: number;
  min: number;
  max: number;
}

export interface ConfigurableAttribute extends Attribute {
  costPerUnit: number;
}

export interface StrategyItem {
  id: number;
  name: string;
  description: string;
  cost: number;
  active: boolean;
}

export interface HintItem {
  id: number;
  name: string;
  description: string;
  reliability: string;
  cost: number;
  used: boolean;
  secret: string;
}

export interface GameEvent {
  id: number;
  name: string;
  description: string;
  imageUrl: string;
  isPositive: boolean;
}

export interface RoundHistory {
  round: number;
  events: GameEvent[];
}

export interface TeamGameState {
  budget: Budget;
  attributes: Attribute[];
  configurable: ConfigurableAttribute[];
  strategies: StrategyItem[];
  hints: HintItem[];
  history: RoundHistory[];
}

// --- Решение команды ---

export interface Report {
  why: string;
  alternative: string;
  risks: string;
  correction: string;
}

export interface Decision {
  attributes: { id: number; value: number }[];
  strategies: number[];
  hints: number[];
  report: Report;
}

export interface DecisionRequest extends Decision {
  round: number;
}

export interface DecisionCost {
  money: number;
  qp: number;
}

// --- Прогнозы (для препода) ---

export interface Prediction {
  event: GameEvent;
  probability: number;
  suggested: boolean;
}

// --- Состояние команды (ответ GET /api/team/state) ---

export interface TeamState {
  game: GameInfo;
  team: TeamSummary | null;
  opening: GameEvent | null;
  state: TeamGameState | null;
  decision: Decision | null;
}

// --- Состояние препода ---

export interface AdminTeam extends TeamSummary {
  token: string;
  state: TeamGameState | null;
  decision: Decision | null;
  cost: DecisionCost | null;
  predictions: Prediction[];
}

export interface AdminState {
  game: GameInfo;
  teams: AdminTeam[];
}

// --- Регистрация ---

export interface RegisterRequest {
  name: string;
  members: string[];
}

export interface RegisterResponse {
  token: string;
  team: TeamSummary;
}

// --- Сценарии ---

export interface Scenario {
  id: string;
}

export interface ScenariosResponse {
  scenarios: Scenario[];
}

// --- Ошибки ---

export interface ApiError {
  error: {
    code: string;
    message: string;
  };
}

// --- История игр ---

export interface GameHistoryItem {
  gameId: number;
  scenarioId: string;
  phase: Phase;
  rounds: number;
  active: boolean;
  createdAt: string;
}

export interface GamesListResponse {
  games: GameHistoryItem[];
}

// --- Детальная история одной игры ---

export interface RoundDetail {
  round: number;
  decision: Decision | null;
  cost: DecisionCost | null;
  auto: boolean;
  predictions: Prediction[];
  events: GameEvent[];
  state: TeamGameState;
}

export interface GameDetailTeam {
  id: number;
  name: string;
  members: string[];
  rounds: RoundDetail[];
  grades: Grade[];
}

export interface GameDetail {
  game: GameHistoryItem;
  teams: GameDetailTeam[];
}

// --- Оценки ---

export type SkillCode =
  | 'problem_statement'
  | 'fact_checking'
  | 'connections'
  | 'alternatives'
  | 'risks'
  | 'revision';

export interface Grade {
  skill: SkillCode;
  score: number;
  comment: string;
}

export interface GradesRequest {
  grades: Grade[];
}

// --- WebSocket ---

export type WsMessageType =
  | 'phase_changed'
  | 'lobby_updated'
  | 'submission_updated'
  | 'team_online'
  | 'team_state';

export interface WsMessage<T = unknown> {
  type: WsMessageType;
  payload: T;
}

export interface LobbyUpdatedPayload {
  teams: TeamSummary[];
}

export interface SubmissionUpdatedPayload {
  teamId: number;
  submitted: boolean;
}

export interface TeamOnlinePayload {
  teamId: number;
  online: boolean;
}

export interface TeamStatePayload {
  round: number;
  events: GameEvent[];
}

// --- Запросы препода ---

export interface StartGameRequest {
  scenarioId: string;
}

export interface CloseRoundRequest {
  round: number;
  force: boolean;
}

export interface EventSelection {
  teamId: number;
  eventIds: number[];
}

export interface ApplyEventsRequest {
  round: number;
  finish: boolean;
  selections: EventSelection[];
}

export interface CreateGameRequest {
  confirm?: boolean;
}

export interface ReadyRequest {
  ready: boolean;
}

