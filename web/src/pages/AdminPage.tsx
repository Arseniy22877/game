import { useEffect, useState, useCallback } from 'react';
import { api, wsManager } from '../api';
import { AdminState, WsMessage } from '../types/api';
import { AdminLogin } from '../components/admin/AdminLogin';
import { AdminLobby } from '../components/admin/AdminLobby';
import { AdminRound } from '../components/admin/AdminRound';
import { AdminFinished } from '../components/admin/AdminFinished';

/**
 * Страница преподавателя (роут /admin).
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

    return () => {
      unsub();
      wsManager.disconnect();
    };
  }, [loggedIn]); // eslint-disable-line react-hooks/exhaustive-deps

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

