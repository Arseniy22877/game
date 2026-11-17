import {
  ApiError,
  AdminState,
  TeamState,
  RegisterRequest,
  RegisterResponse,
  DecisionRequest,
  DecisionCost,
  ScenariosResponse,
  StartGameRequest,
  CloseRoundRequest,
  ApplyEventsRequest,
  CreateGameRequest,
  ReadyRequest,
  GameInfo,
  GamesListResponse,
  GameDetail,
  GradesRequest,
} from '../types/api';

const API_BASE = '/api';

function getStorageItem(key: string): string | null {
  try {
    if (typeof sessionStorage !== 'undefined') {
      const val = sessionStorage.getItem(key);
      if (val) return val;
    }
  } catch {
    // ignore storage restrictions
  }
  try {
    if (typeof localStorage !== 'undefined') {
      return localStorage.getItem(key);
    }
  } catch {
    // ignore storage restrictions
  }
  return null;
}

function setStorageItem(key: string, val: string | null): void {
  if (val) {
    try { sessionStorage.setItem(key, val); } catch { /* ignore */ }
    try { localStorage.setItem(key, val); } catch { /* ignore */ }
  } else {
    try { sessionStorage.removeItem(key); } catch { /* ignore */ }
    try { localStorage.removeItem(key); } catch { /* ignore */ }
  }
}

class ApiClient {
  private teamToken: string | null = null;
  private adminToken: string | null = null;

  // --- Токены ---

  setTeamToken(token: string | null) {
    this.teamToken = token;
    setStorageItem('teamToken', token);
  }

  getTeamToken(): string | null {
    if (!this.teamToken) {
      this.teamToken = getStorageItem('teamToken');
    }
    return this.teamToken;
  }

  setAdminToken(token: string) {
    this.adminToken = token;
    setStorageItem('adminToken', token);
  }

  getAdminToken(): string | null {
    if (!this.adminToken) {
      this.adminToken = getStorageItem('adminToken');
    }
    return this.adminToken;
  }

  clearTeamToken() {
    this.teamToken = null;
    setStorageItem('teamToken', null);
  }

  // --- Утилита ---

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
    token?: string | null,
  ): Promise<T> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    const res = await fetch(`${API_BASE}${path}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });

    if (!res.ok) {
      let apiErr: ApiError;
      try {
        apiErr = await res.json();
      } catch {
        throw new ApiRequestError(res.status, 'internal', res.statusText);
      }
      throw new ApiRequestError(res.status, apiErr.error.code, apiErr.error.message);
    }

    return res.json();
  }

  private teamRequest<T>(method: string, path: string, body?: unknown): Promise<T> {
    return this.request<T>(method, path, body, this.getTeamToken());
  }

  private adminRequest<T>(method: string, path: string, body?: unknown): Promise<T> {
    return this.request<T>(method, path, body, this.getAdminToken());
  }

  // --- Публичное ---

  getGameInfo(): Promise<GameInfo> {
    return this.request<GameInfo>('GET', '/game');
  }

  // --- Команда ---

  registerTeam(data: RegisterRequest): Promise<RegisterResponse> {
    return this.request<RegisterResponse>('POST', '/teams', data);
  }

  setReady(data: ReadyRequest) {
    return this.teamRequest('PUT', '/team/ready', data);
  }

  getTeamState(): Promise<TeamState> {
    return this.teamRequest<TeamState>('GET', '/team/state');
  }

  submitDecision(data: DecisionRequest): Promise<{ cost: DecisionCost }> {
    return this.teamRequest<{ cost: DecisionCost }>('POST', '/team/decision', data);
  }

  // --- Преподаватель ---

  getScenarios(): Promise<ScenariosResponse> {
    return this.adminRequest<ScenariosResponse>('GET', '/admin/scenarios');
  }

  createGame(data?: CreateGameRequest): Promise<AdminState> {
    return this.adminRequest<AdminState>('POST', '/admin/game', data);
  }

  deleteTeam(teamId: number): Promise<AdminState> {
    return this.adminRequest<AdminState>('DELETE', `/admin/teams/${teamId}`);
  }

  getAdminState(): Promise<AdminState> {
    return this.adminRequest<AdminState>('GET', '/admin/state');
  }

  startGame(data: StartGameRequest): Promise<AdminState> {
    return this.adminRequest<AdminState>('POST', '/admin/game/start', data);
  }

  closeRound(data: CloseRoundRequest): Promise<AdminState> {
    return this.adminRequest<AdminState>('POST', '/admin/round/close', data);
  }

  applyEvents(data: ApplyEventsRequest): Promise<AdminState> {
    return this.adminRequest<AdminState>('POST', '/admin/round/events', data);
  }

  finishGame(): Promise<AdminState> {
    return this.adminRequest<AdminState>('POST', '/admin/game/finish');
  }

  getGames(): Promise<GamesListResponse> {
    return this.adminRequest<GamesListResponse>('GET', '/admin/games');
  }

  getGameDetail(gameId: number): Promise<GameDetail> {
    return this.adminRequest<GameDetail>('GET', `/admin/games/${gameId}`);
  }

  setGrades(gameId: number, teamId: number, data: GradesRequest): Promise<GradesRequest> {
    return this.adminRequest<GradesRequest>(
      'PUT',
      `/admin/games/${gameId}/teams/${teamId}/grades`,
      data,
    );
  }
}

// Кастомный класс ошибки для удобной обработки
export class ApiRequestError extends Error {
  constructor(
    public status: number,
    public code: string,
    public userMessage: string,
  ) {
    super(userMessage);
    this.name = 'ApiRequestError';
  }
}

export const api = new ApiClient();

