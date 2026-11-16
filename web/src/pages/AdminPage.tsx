import { useEffect, useState, useCallback } from 'react';
import { api, wsManager } from '../api';
import { AdminState, WsMessage } from '../types/api';
import { AdminLogin } from '../components/admin/AdminLogin';
import { AdminLobby } from '../components/admin/AdminLobby';
import { AdminRound } from '../components/admin/AdminRound';
import { AdminFinished } from '../components/admin/AdminFinished';

/**
 * Страница преподавателя (роут /admin).
 * Синхронизирует состояние в реальном времени через WebSocket + авто-опрос.
 */
export function AdminPage() {
  const [state, setState] = useState<AdminState | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [loggedIn, setLoggedIn] = useState(!!api.getAdminToken());

  const loadState = useCallback(async () => {
    try {
      const adminState = await api.getAdminState();
      setState(adminState);
      setError(null);
    } catch (e: unknown) {
      const err = e as Error & { code?: string; userMessage?: string };
      if (err.code === 'unauthorized') {
        setLoggedIn(false);
      } else {
        setError(err.userMessage || err.message);
      }
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    if (!loggedIn) {
      setLoading(false);
      return;
    }

    loadState();

    const token = api.getAdminToken();
    wsManager.connect(token);

    const unsub = wsManager.subscribe((msg: WsMessage) => {
      switch (msg.type) {
        case 'phase_changed':
        case 'lobby_updated':
        case 'submission_updated':
        case 'team_online':
          loadState();
          break;
      }
    });

    // Регулярный опрос для надежности
    const pollInterval = setInterval(() => {
      loadState();
    }, 2500);

    return () => {
      unsub();
      clearInterval(pollInterval);
      wsManager.disconnect();
    };
  }, [loggedIn, loadState]);

  if (!loggedIn) {
    return (
      <AdminLogin
        onLogin={(token) => {
          api.setAdminToken(token);
          setLoggedIn(true);
          setLoading(true);
        }}
      />
    );
  }

  if (loading && !state) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-50">
        <div className="flex flex-col items-center gap-3">
          <div className="w-10 h-10 border-4 border-slate-300 border-t-slate-800 rounded-full animate-spin" />
          <div className="text-sm font-medium text-slate-500">Загрузка панели...</div>
        </div>
      </div>
    );
  }

  if (error && !state) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-50 px-4">
        <div className="max-w-md w-full bg-white rounded-2xl shadow-sm border border-slate-200 p-8 text-center">
          <div className="w-12 h-12 mx-auto mb-4 rounded-full bg-red-100 flex items-center justify-center text-red-600 font-bold text-xl">
            ✕
          </div>
          <h2 className="text-lg font-bold text-slate-800 mb-2">Ошибка подключения</h2>
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

  const phase = state.game.phase;

  if (phase === 'none' || phase === 'lobby') {
    return <AdminLobby state={state} onUpdate={loadState} />;
  }

  if (phase === 'finished') {
    return <AdminFinished state={state} onUpdate={loadState} />;
  }

  // round_open или round_closed
  return <AdminRound state={state} onUpdate={loadState} />;
}
