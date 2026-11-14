import { useEffect, useState, useCallback } from 'react';
import { api, wsManager } from '../api';
import { TeamState, WsMessage, GameInfo } from '../types/api';
import { TeamLobby } from '../components/team/TeamLobby';
import { TeamGame } from '../components/team/TeamGame';
import { TeamFinished } from '../components/team/TeamFinished';
import { TeamWaiting } from '../components/team/TeamWaiting';

/**
 * Страница команды (корневой роут /).
 * Переключает экраны по фазе игры и наличию токена.
 */
export function TeamPage() {
  const [state, setState] = useState<TeamState | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // При загрузке проверяем token из URL (вход с другого устройства)
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const urlToken = params.get('token');
    if (urlToken) {
      api.setTeamToken(urlToken);
      // Убираем token из URL
      window.history.replaceState({}, '', window.location.pathname);
    }
  }, []);

  const loadState = useCallback(async () => {
    const token = api.getTeamToken();
    if (!token) {
      // Нет токена: получаем GameInfo, чтобы показать лобби или ожидание
      try {
        const gameInfo = await api.getGameInfo();
        setState({
          game: gameInfo,
          team: null,
          opening: null,
          state: null,
          decision: null,
        });
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
        // Токен от прошлой игры — сбрасываем
        api.clearTeamToken();
        const gameInfo = await api.getGameInfo();
        setState({
          game: gameInfo,
          team: null,
          opening: null,
          state: null,
          decision: null,
        });
      } else {
        setError(err.userMessage || err.message);
      }
    }
    setLoading(false);
  }, []);

  // Подключаем WS и загружаем состояние
  useEffect(() => {
    loadState();

    const token = api.getTeamToken();
    wsManager.connect(token);

    const unsub = wsManager.subscribe((msg: WsMessage) => {
      switch (msg.type) {
        case 'phase_changed': {
          const info = msg.payload as GameInfo;
          // Если gameId изменился — сбрасываем токен
          if (state && state.game && info.gameId !== state.game.gameId) {
            api.clearTeamToken();
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

    return () => {
      unsub();
      wsManager.disconnect();
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-lg text-gray-500">Загрузка...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <div className="text-red-500 text-lg mb-4">{error}</div>
          <button
            onClick={() => { setLoading(true); setError(null); loadState(); }}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"
          >
            Повторить
          </button>
        </div>
      </div>
    );
  }

  if (!state) return null;

  const { game, team } = state;
  const hasToken = !!api.getTeamToken();
  const phase = game.phase;

  // Нет игры — ждём
  if (phase === 'none') {
    return <TeamWaiting message="Ожидание создания игры преподавателем..." />;
  }

  // Лобби — если нет токена, показываем форму регистрации
  if (phase === 'lobby') {
    return <TeamLobby team={team} onRegistered={loadState} />;
  }

  // Игра идёт, но нет токена — опоздали
  if (!hasToken) {
    return <TeamWaiting message="Игра уже началась. Попросите ссылку для входа у преподавателя." />;
  }

  // Игра завершена
  if (phase === 'finished') {
    return <TeamFinished state={state} />;
  }

  // round_open или round_closed — основной игровой экран
  return <TeamGame state={state} onUpdate={loadState} />;
}

