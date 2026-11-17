import { useEffect, useState, useCallback, useRef } from 'react';
import { api, wsManager } from '../api';
import { TeamState, WsMessage, GameInfo } from '../types/api';
import { TeamLobby } from '../components/team/TeamLobby';
import { TeamGame } from '../components/team/TeamGame';
import { TeamFinished } from '../components/team/TeamFinished';
import { TeamWaiting } from '../components/team/TeamWaiting';

/**
 * Страница команды (корневой роут /).
 * Автоматически обновляет состояние через WebSocket + периодический fallback polling.
 */
export function TeamPage() {
  const [state, setState] = useState<TeamState | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const stateRef = useRef<TeamState | null>(null);
  stateRef.current = state;

  // При загрузке проверяем token из URL (вход со второго устройства)
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const urlToken = params.get('token');
    if (urlToken) {
      api.setTeamToken(urlToken);
      window.history.replaceState({}, '', window.location.pathname);
    }
  }, []);

  const loadState = useCallback(async () => {
    const token = api.getTeamToken();
    if (!token) {
      // Нет токена: получаем GameInfo для лобби
      try {
        const gameInfo = await api.getGameInfo();
        setState({
          game: gameInfo,
          team: null,
          opening: null,
          state: null,
          decision: null,
        });
        setError(null);
      } catch (e: unknown) {
        const err = e as Error & { userMessage?: string };
        setError(err.userMessage || err.message);
      }
      setLoading(false);
      return;
    }

    try {
      const teamState = await api.getTeamState();
      setState(teamState);
      setError(null);
    } catch (e: unknown) {
      const err = e as Error & { code?: string; userMessage?: string };
      if (err.code === 'unauthorized') {
        // Токен от старой игры — сбрасываем и подключаемся как гость
        api.clearTeamToken();
        wsManager.connect(null);
        try {
          const gameInfo = await api.getGameInfo();
          setState({
            game: gameInfo,
            team: null,
            opening: null,
            state: null,
            decision: null,
          });
        } catch {
          // ignore
        }
      } else {
        setError(err.userMessage || err.message);
      }
    }
    setLoading(false);
  }, []);

  // Подключение WebSocket и синхронизация
  useEffect(() => {
    loadState();

    const currentToken = api.getTeamToken();
    wsManager.connect(currentToken);

    const unsub = wsManager.subscribe((msg: WsMessage) => {
      switch (msg.type) {
        case 'phase_changed': {
          const info = msg.payload as GameInfo;
          // Если игра сменилась — сбрасываем токен
          const curr = stateRef.current;
          if (curr && curr.game && info.gameId !== curr.game.gameId) {
            api.clearTeamToken();
            wsManager.connect(null);
          }
          loadState();
          break;
        }
        case 'lobby_updated':
        case 'submission_updated':
        case 'team_state':
          loadState();
          break;
      }
    });

    // Регулярный fallback polling каждые 2.5 секунды для 100% надёжности
    const pollInterval = setInterval(() => {
      loadState();
    }, 2500);

    return () => {
      unsub();
      clearInterval(pollInterval);
      wsManager.disconnect();
    };
  }, [loadState]);

  const handleRegistered = useCallback(() => {
    const token = api.getTeamToken();
    wsManager.connect(token);
    loadState();
  }, [loadState]);

  if (loading && !state) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-50">
        <div className="flex flex-col items-center gap-3">
          <div className="w-10 h-10 border-4 border-slate-300 border-t-slate-800 rounded-full animate-spin" />
          <div className="text-sm font-medium text-slate-500">Загрузка игры...</div>
        </div>
      </div>
    );
  }

  if (error && !state) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-50 px-4">
        <div className="max-w-md w-full bg-white rounded-2xl shadow-sm border border-slate-200 p-8 text-center">
          <div className="w-12 h-12 mx-auto mb-4 rounded-2xl bg-rose-100 flex items-center justify-center text-rose-600">
            <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
          </div>
          <h2 className="text-lg font-bold text-slate-800 mb-2">Не удалось подключиться</h2>
          <div className="text-red-600 text-sm mb-6">{error}</div>
          <button
            onClick={() => { setLoading(true); setError(null); loadState(); }}
            className="w-full py-2.5 bg-slate-900 text-white text-sm font-semibold rounded-xl hover:bg-slate-800 transition-colors shadow-sm"
          >
            Повторить попытку
          </button>
        </div>
      </div>
    );
  }

  if (!state) return null;

  const { game, team } = state;
  const hasToken = !!api.getTeamToken();
  const phase = game.phase;

  // Нет активной игры
  if (phase === 'none') {
    return <TeamWaiting message="Ожидание создания игры преподавателем..." />;
  }

  // Лобби: регистрация или ожидание старта
  if (phase === 'lobby') {
    return <TeamLobby team={team} onRegistered={handleRegistered} />;
  }

  // Игра началась, но токена нет
  if (!hasToken || !team) {
    return (
      <TeamWaiting
        message="Игра уже началась. Если ваша команда уже в игре, перейдите по ссылке от преподавателя или введите токен ниже."
        onEnterToken={(t) => {
          api.setTeamToken(t);
          loadState();
        }}
      />
    );
  }

  // Игра завершена
  if (phase === 'finished') {
    return <TeamFinished state={state} />;
  }

  // Игровой стол (round_open или round_closed)
  return <TeamGame key={`round-${game.round}`} state={state} onUpdate={loadState} />;
}
