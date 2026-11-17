import { useState, useEffect } from 'react';
import { api } from '../../api';
import { AdminState, TeamSummary } from '../../types/api';

interface Props {
  state: AdminState;
  onUpdate: () => void;
}

/**
 * Экран 1: Лобби преподавателя.
 * В точности воспроизводит страницу 1 PDF:
 * - Выбор сценария
 * - Карточки команд со списком участников и индикатором готовности (V / X)
 * - Кнопка «Начать»
 */
export function AdminLobby({ state, onUpdate }: Props) {
  const { game, teams } = state;
  const isNone = game.phase === 'none';

  const [scenarios, setScenarios] = useState<{ id: string }[]>([]);
  const [selectedScenario, setSelectedScenario] = useState('demo');
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
    if (!confirm('Удалить команду из лобби?')) return;
    try {
      await api.deleteTeam(teamId);
      onUpdate();
    } catch (e: unknown) {
      const err = e as Error & { userMessage?: string };
      setError(err.userMessage || err.message);
    }
  };

  // Гарантированная дедупликация команд по уникальному ID
  const uniqueTeams = Array.from(new Map(teams.map((t) => [t.id, t])).values());
  const allReady = uniqueTeams.length > 0 && uniqueTeams.every((t) => t.ready);

  return (
    <div className="min-h-screen bg-slate-50 flex flex-col">
      {/* Шапка преподавателя */}
      <header className="bg-white border-b border-slate-200">
        <div className="max-w-5xl mx-auto px-4 py-4 flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold text-slate-900">Панель управления игрой</h1>
            <p className="text-xs text-slate-500">Режим преподавателя / Фасилитатор</p>
          </div>
          <div className="flex items-center gap-3">
            <span className="inline-flex items-center gap-1.5 px-3 py-1 bg-amber-50 border border-amber-200 text-amber-700 text-xs font-semibold rounded-full">
              <span className="w-2 h-2 rounded-full bg-amber-500 animate-pulse" />
              Фаза: Лобби
            </span>
          </div>
        </div>
      </header>

      <main className="max-w-5xl w-full mx-auto px-4 py-8 space-y-6 flex-1">
        {error && (
          <div className="p-4 bg-red-50 border border-red-200 rounded-2xl text-red-700 text-sm">
            {error}
          </div>
        )}

        {/* Выбор сценария */}
        <div className="bg-white rounded-3xl border border-slate-200 p-6 shadow-sm">
          <label className="block text-xs font-semibold text-slate-500 uppercase tracking-wider mb-2">
            Сценарий симуляции
          </label>
          <div className="flex items-center gap-4">
            <select
              value={selectedScenario}
              onChange={(e) => setSelectedScenario(e.target.value)}
              className="flex-1 px-4 py-3 bg-slate-50 border border-slate-200 rounded-2xl text-slate-900 font-semibold focus:bg-white focus:ring-2 focus:ring-slate-900 focus:border-transparent outline-none transition-all cursor-pointer"
            >
              {scenarios.length === 0 ? (
                <option value="demo">demo (Базовый сценарий)</option>
              ) : (
                scenarios.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.id}
                  </option>
                ))
              )}
            </select>

            {isNone && (
              <button
                onClick={handleCreateGame}
                disabled={loading}
                className="px-6 py-3 bg-slate-900 hover:bg-slate-800 text-white font-semibold rounded-2xl transition-all shadow-sm"
              >
                Создать игру
              </button>
            )}
          </div>
        </div>

        {/* Сетка команд (как на макете в PDF) */}
        <div className="bg-white rounded-3xl border border-slate-200 p-6 shadow-sm">
          <div className="flex items-center justify-between mb-6">
            <div>
              <h2 className="text-lg font-bold text-slate-900">Команды в игре</h2>
              <p className="text-xs text-slate-500">
                Зарегистрировано: {uniqueTeams.length} | Готовы: {uniqueTeams.filter((t) => t.ready).length}
              </p>
            </div>
            {uniqueTeams.length > 0 && !allReady && (
              <span className="text-xs font-medium text-amber-600 bg-amber-50 px-3 py-1 rounded-full border border-amber-200">
                Ожидание готовности всех команд
              </span>
            )}
          </div>

          {uniqueTeams.length === 0 ? (
            <div className="text-center py-16 border-2 border-dashed border-slate-200 rounded-3xl">
              <div className="w-12 h-12 mx-auto mb-3 rounded-2xl bg-slate-100 flex items-center justify-center text-slate-400">
                <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                </svg>
              </div>
              <div className="text-sm font-semibold text-slate-700">Ожидание подключения команд</div>
              <div className="text-xs text-slate-400 mt-1 max-w-sm mx-auto">
                Студенты могут открыть главную страницу сайта на своих устройствах и зарегистрировать команду.
              </div>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {uniqueTeams.map((t) => (
                <TeamLobbyCard
                  key={t.id}
                  team={t}
                  onDelete={() => handleDeleteTeam(t.id)}
                />
              ))}
            </div>
          )}
        </div>

        {/* Кнопка "Начать" */}
        <div className="pt-2">
          <button
            onClick={handleStart}
            disabled={loading || !allReady || isNone}
            className={`w-full py-4 text-base font-bold rounded-2xl shadow-sm transition-all ${
              allReady && !isNone
                ? 'bg-slate-900 text-white hover:bg-slate-800'
                : 'bg-slate-200 text-slate-400 cursor-not-allowed'
            }`}
          >
            {loading ? 'Запуск симуляции...' : 'Начать'}
          </button>
          {!allReady && uniqueTeams.length > 0 && (
            <div className="text-center text-xs text-slate-400 mt-2">
              Для запуска игры необходимо, чтобы все зарегистрированные команды были в статусе «Готовы»
            </div>
          )}
        </div>
      </main>
    </div>
  );
}

function TeamLobbyCard({
  team,
  onDelete,
}: {
  team: TeamSummary & { token?: string };
  onDelete: () => void;
}) {
  const [showToken, setShowToken] = useState(false);

  return (
    <div className="border border-slate-200 rounded-2xl overflow-hidden bg-white shadow-sm flex flex-col justify-between">
      {/* Шапка карточки команды */}
      <div className="px-5 py-4 border-b border-slate-100 flex items-center justify-between bg-slate-50/50">
        <div className="flex items-center gap-2">
          <span className="font-bold text-slate-900">{team.name}</span>
          <span
            className={`w-2 h-2 rounded-full ${
              team.online ? 'bg-emerald-500' : 'bg-slate-300'
            }`}
            title={team.online ? 'Онлайн' : 'Офлайн'}
          />
        </div>
        <div className="flex items-center gap-2">
          {team.ready ? (
            <span className="inline-flex items-center justify-center w-7 h-7 rounded-full bg-emerald-100 text-emerald-600 font-bold text-sm">
              ✓
            </span>
          ) : (
            <span className="inline-flex items-center justify-center w-7 h-7 rounded-full bg-rose-100 text-rose-500 font-bold text-sm">
              ✕
            </span>
          )}
          <button
            type="button"
            onClick={onDelete}
            className="text-slate-400 hover:text-rose-500 p-1 text-sm font-semibold transition-colors"
            title="Удалить команду"
          >
            ✕
          </button>
        </div>
      </div>

      {/* Список участников */}
      <div className="p-5 space-y-2 flex-1">
        <div className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-2">
          Участники ({team.members.length})
        </div>
        {team.members.map((m, idx) => (
          <div key={idx} className="text-sm font-medium text-slate-700 py-1 border-b border-slate-100 last:border-none">
            {m}
          </div>
        ))}
      </div>

      {/* Подвал с токеном / ссылкой для входа */}
      {team.token && (
        <div className="px-5 py-2.5 bg-slate-50 border-t border-slate-100 flex items-center justify-between text-xs">
          <button
            type="button"
            onClick={() => setShowToken(!showToken)}
            className="text-slate-500 hover:text-slate-800 font-medium inline-flex items-center gap-1.5"
          >
            <svg className="w-3.5 h-3.5 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
            </svg>
            <span>{showToken ? 'Скрыть ссылку' : 'Ссылка для входа'}</span>
          </button>
          {showToken && (
            <span className="font-mono text-slate-600 select-all">
              /?token={team.token}
            </span>
          )}
        </div>
      )}
    </div>
  );
}
