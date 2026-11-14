import { useState, useEffect } from 'react';
import { api } from '../../api';
import { AdminState, TeamSummary } from '../../types/api';

interface Props {
  state: AdminState;
  onUpdate: () => void;
}

/**
 * Лобби преподавателя: создание игры, выбор сценария, список команд, старт.
 */
export function AdminLobby({ state, onUpdate }: Props) {
  const { game, teams } = state;
  const isNone = game.phase === 'none';
  const isLobby = game.phase === 'lobby';

  const [scenarios, setScenarios] = useState<{ id: string }[]>([]);
  const [selectedScenario, setSelectedScenario] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.getScenarios().then((res) => {
      setScenarios(res.scenarios);
      if (res.scenarios.length > 0 && !selectedScenario) {
        setSelectedScenario(res.scenarios[0].id);
      }
    }).catch(() => {});
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const handleCreateGame = async () => {
    setLoading(true);
    setError(null);
    try {
      await api.createGame();
      onUpdate();
    } catch (e: unknown) {
      const err = e as Error & { code?: string; userMessage?: string };
      if (err.code === 'game_in_progress') {
        if (confirm('Идёт активная игра. Завершить её и создать новую?')) {
          try {
            await api.createGame({ confirm: true });
            onUpdate();
          } catch (e2: unknown) {
            const err2 = e2 as Error & { userMessage?: string };
            setError(err2.userMessage || err2.message);
          }
        }
      } else {
        setError(err.userMessage || err.message);
      }
    }
    setLoading(false);
  };

  const handleStart = async () => {
    if (!selectedScenario) return;
    setLoading(true);
    setError(null);
    try {
      await api.startGame({ scenarioId: selectedScenario });
      onUpdate();
    } catch (e: unknown) {
      const err = e as Error & { userMessage?: string };
      setError(err.userMessage || err.message);
    }
    setLoading(false);
  };

  const handleDeleteTeam = async (teamId: number) => {
    if (!confirm('Удалить команду?')) return;
    try {
      await api.deleteTeam(teamId);
      onUpdate();
    } catch (e: unknown) {
      const err = e as Error & { userMessage?: string };
      setError(err.userMessage || err.message);
    }
  };

  const allReady = teams.length > 0 && teams.every((t) => t.ready);

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="bg-white shadow-sm border-b">
        <div className="max-w-4xl mx-auto px-4 py-4">
          <h1 className="text-xl font-bold">Панель преподавателя</h1>
          <span className="text-sm text-gray-500">
            {isNone ? 'Нет активной игры' : 'Лобби'}
          </span>
        </div>
      </header>

      <main className="max-w-4xl mx-auto px-4 py-6 space-y-6">
        {error && (
          <div className="p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">
            {error}
          </div>
        )}

        {/* Создание / настройка игры */}
        <section className="bg-white rounded-xl border p-6">
          <h2 className="text-lg font-semibold mb-4">
            {isNone ? 'Создать игру' : 'Настройка игры'}
          </h2>

          {isNone && (
            <button
              onClick={handleCreateGame}
              disabled={loading}
              className="px-6 py-3 bg-blue-600 text-white font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50"
            >
              Создать новую игру
            </button>
          )}

          {isLobby && (
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Сценарий
                </label>
                <select
                  value={selectedScenario}
                  onChange={(e) => setSelectedScenario(e.target.value)}
                  className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 outline-none"
                >
                  {scenarios.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.id}
                    </option>
                  ))}
                </select>
              </div>

              <button
                onClick={handleStart}
                disabled={loading || !allReady}
                className="px-6 py-3 bg-green-600 text-white font-medium rounded-lg hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {!allReady
                  ? teams.length === 0
                    ? 'Ожидание команд...'
                    : 'Не все команды готовы'
                  : 'Начать игру'}
              </button>
            </div>
          )}
        </section>

        {/* Список команд */}
        {isLobby && (
          <section className="bg-white rounded-xl border p-6">
            <h2 className="text-lg font-semibold mb-4">
              Команды ({teams.length})
            </h2>
            {teams.length === 0 ? (
              <p className="text-gray-500">Пока никто не зарегистрировался</p>
            ) : (
              <div className="space-y-3">
                {teams.map((team) => (
                  <TeamCard
                    key={team.id}
                    team={team}
                    onDelete={() => handleDeleteTeam(team.id)}
                  />
                ))}
              </div>
            )}
          </section>
        )}
      </main>
    </div>
  );
}

function TeamCard({ team, onDelete }: { team: TeamSummary & { token?: string }; onDelete: () => void }) {
  const [showToken, setShowToken] = useState(false);
  const token = team.token;

  return (
    <div className="flex items-center justify-between p-4 bg-gray-50 rounded-lg">
      <div className="flex items-center gap-3">
        <div
          className={`w-3 h-3 rounded-full ${
            team.ready ? 'bg-green-500' : 'bg-yellow-400'
          }`}
        />
        <div>
          <div className="font-medium">{team.name}</div>
          <div className="text-sm text-gray-500">{team.members.join(', ')}</div>
          {token && showToken && (
            <div className="text-xs text-gray-400 mt-1 font-mono break-all">{token}</div>
          )}
        </div>
      </div>
      <div className="flex items-center gap-2">
        <span
          className={`text-xs px-2 py-1 rounded-full ${
            team.online ? 'bg-green-100 text-green-700' : 'bg-gray-200 text-gray-500'
          }`}
        >
          {team.online ? 'онлайн' : 'офлайн'}
        </span>
        {token && (
          <button
            onClick={() => setShowToken(!showToken)}
            className="text-xs text-blue-600 hover:text-blue-800"
            title="Показать токен для входа"
          >
            🔑
          </button>
        )}
        <button
          onClick={onDelete}
          className="text-red-400 hover:text-red-600"
          title="Удалить команду"
        >
          ✕
        </button>
      </div>
    </div>
  );
}

