import { useState, useEffect, useRef } from 'react';
import { api } from '../../api';
import { AdminState } from '../../types/api';

interface Props {
  state: AdminState;
  onUpdate: () => void;
}

/**
 * Экран 4: Панель преподавателя во время раунда.
 * В точности воспроизводит страницу 2 и 3 PDF:
 * - Список команд с крестиками и галочками (кто завершил раунд, а кто нет)
 * - Кнопка «Завершить раунд» (завершает раунд, не начиная следующий)
 * - В фазе round_closed каждая команда кликабельна:
 *   внутри информация о выборе на этот раунд и рассчитанные возможные события с вероятностями
 * - Преподаватель кликает по событиям для фиксации
 * - Переход к следующему раунду или завершение игры
 */
export function AdminRound({ state, onUpdate }: Props) {
  const { game, teams } = state;
  const isOpen = game.phase === 'round_open';
  const isClosed = game.phase === 'round_closed';

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedTeamId, setSelectedTeamId] = useState<number | null>(teams.length > 0 ? teams[0].id : null);

  // Выбранные события для команд: { [teamId]: Set(eventIds) }
  const [eventSelections, setEventSelections] = useState<Record<number, Set<number>>>({});
  const initializedRoundRef = useRef<number | null>(null);

  // Автоматическая инициализация рекомендованных событий СТРОГО 1 РАЗ на каждый вход в round_closed
  useEffect(() => {
    if (isClosed) {
      const hasPredictions = teams.some((t) => (t.predictions || []).length > 0);
      if (hasPredictions && initializedRoundRef.current !== game.round) {
        const initial: Record<number, Set<number>> = {};
        teams.forEach((t) => {
          const suggested = (t.predictions || [])
            .filter((p) => p.suggested)
            .map((p) => p.event.id);
          initial[t.id] = new Set(suggested);
        });
        setEventSelections(initial);
        initializedRoundRef.current = game.round;
      }
    } else {
      initializedRoundRef.current = null;
    }
  }, [isClosed, game.round, teams]);

  const handleCloseRound = async (force: boolean) => {
    const notSubmitted = teams.filter((t) => !t.submitted && !t.bankrupt);
    if (!force && notSubmitted.length > 0) {
      const names = notSubmitted.map((t) => t.name).join(', ');
      if (!confirm(`Команды не сдали решение: ${names}. Завершить сбор решений принудительно?`)) {
        return;
      }
      force = true;
    }

    if (!force && !confirm('Завершить раунд? После этого команды не смогут менять решения.')) return;

    setLoading(true);
    setError(null);
    try {
      await api.closeRound({ round: game.round, force });
      onUpdate();
    } catch (e: unknown) {
      const err = e as Error & { userMessage?: string };
      setError(err.userMessage || err.message);
    }
    setLoading(false);
  };

  const toggleEvent = (teamId: number, eventId: number) => {
    setEventSelections((prev) => {
      const current = new Set(prev[teamId] || []);
      if (current.has(eventId)) {
        current.delete(eventId);
      } else {
        current.add(eventId);
      }
      return {
        ...prev,
        [teamId]: current,
      };
    });
  };

  const selectSuggested = (teamId: number) => {
    const t = teams.find((x) => x.id === teamId);
    if (!t) return;
    const suggested = (t.predictions || [])
      .filter((p) => p.suggested)
      .map((p) => p.event.id);
    setEventSelections((prev) => ({
      ...prev,
      [teamId]: new Set(suggested),
    }));
  };

  const clearEvents = (teamId: number) => {
    setEventSelections((prev) => ({
      ...prev,
      [teamId]: new Set(),
    }));
  };

  const handleApplyEvents = async (finish: boolean) => {
    const action = finish ? 'Завершить игру' : 'Применить события и открыть следующий раунд';
    if (!confirm(`${action}? Это действие зафиксирует выбранные события для всех команд.`)) return;

    setLoading(true);
    setError(null);
    try {
      await api.applyEvents({
        round: game.round,
        finish,
        selections: teams.map((t) => ({
          teamId: t.id,
          eventIds: Array.from(eventSelections[t.id] || []),
        })),
      });
      onUpdate();
    } catch (e: unknown) {
      const err = e as Error & { userMessage?: string };
      setError(err.userMessage || err.message);
    }
    setLoading(false);
  };

  const handleFinishGame = async () => {
    if (!confirm('Завершить игру сейчас? Решения текущего раунда будут отброшены.')) return;
    setLoading(true);
    try {
      await api.finishGame();
      onUpdate();
    } catch (e: unknown) {
      const err = e as Error & { userMessage?: string };
      setError(err.userMessage || err.message);
    }
    setLoading(false);
  };

  const selectedTeam = teams.find((t) => t.id === selectedTeamId) || teams[0];
  const allSubmitted = teams.every((t) => t.submitted || t.bankrupt);

  return (
    <div className="min-h-screen bg-slate-50 flex flex-col">
      {/* Шапка преподавателя */}
      <header className="bg-white border-b border-slate-200 sticky top-0 z-20">
        <div className="max-w-7xl mx-auto px-4 py-3.5 flex items-center justify-between">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl font-bold text-slate-900">Раунд {game.round}</h1>
              <span className={`px-3 py-1 rounded-full text-xs font-semibold border ${
                isOpen
                  ? 'bg-amber-50 text-amber-700 border-amber-200'
                  : 'bg-indigo-50 text-indigo-700 border-indigo-200'
              }`}>
                {isOpen ? 'Сбор решений игроков' : 'Выбор событий раунда'}
              </span>
            </div>
            <p className="text-xs text-slate-400 mt-0.5">
              Сценарий: {game.scenarioId} | Сдано решений: {teams.filter((t) => t.submitted).length} / {teams.length}
            </p>
          </div>

          <div className="flex items-center gap-3">
            {isOpen && (
              <>
                <button
                  type="button"
                  onClick={() => handleCloseRound(false)}
                  disabled={loading}
                  className="px-5 py-2.5 bg-slate-900 hover:bg-slate-800 text-white text-sm font-bold rounded-2xl shadow-sm transition-all disabled:opacity-50"
                >
                  Завершить раунд
                </button>
                <button
                  type="button"
                  onClick={handleFinishGame}
                  disabled={loading}
                  className="px-4 py-2.5 text-rose-600 hover:bg-rose-50 text-sm font-semibold rounded-2xl transition-all"
                >
                  Завершить игру
                </button>
              </>
            )}

            {isClosed && (
              <>
                <button
                  type="button"
                  onClick={() => handleApplyEvents(false)}
                  disabled={loading}
                  className="px-5 py-2.5 bg-slate-900 hover:bg-slate-800 text-white text-sm font-bold rounded-2xl shadow-sm transition-all disabled:opacity-50"
                >
                  Применить события и начать следующий раунд →
                </button>
                <button
                  type="button"
                  onClick={() => handleApplyEvents(true)}
                  disabled={loading}
                  className="px-4 py-2.5 bg-rose-600 hover:bg-rose-700 text-white text-sm font-bold rounded-2xl shadow-sm transition-all disabled:opacity-50"
                >
                  Завершить игру
                </button>
              </>
            )}
          </div>
        </div>
      </header>

      {/* Ошибки */}
      {error && (
        <div className="max-w-7xl mx-auto px-4 mt-6 w-full">
          <div className="p-4 bg-rose-50 border border-rose-200 rounded-2xl text-rose-700 text-sm font-medium">
            {error}
          </div>
        </div>
      )}

      {/* Основной двухколоночный контент */}
      <main className="max-w-7xl w-full mx-auto px-4 py-8 flex-1">
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 items-start">
          {/* Левая колонка: Список команд со статусами (4 колонки) */}
          <div className="lg:col-span-4 space-y-4">
            <div className="flex items-center justify-between">
              <h2 className="text-base font-bold text-slate-900">Команды</h2>
              <span className="text-xs text-slate-400">
                {allSubmitted ? 'Все сдали ✓' : 'Ожидание сдачи...'}
              </span>
            </div>

            <div className="space-y-3">
              {teams.map((t) => {
                const isSelected = selectedTeam?.id === t.id;
                const fixedEventsCount = (eventSelections[t.id] || new Set()).size;

                return (
                  <button
                    key={t.id}
                    type="button"
                    onClick={() => setSelectedTeamId(t.id)}
                    className={`w-full text-left p-5 rounded-3xl border transition-all flex items-center justify-between ${
                      isSelected
                        ? 'bg-white border-slate-900 ring-2 ring-slate-900 shadow-md'
                        : 'bg-white border-slate-200 hover:border-slate-300 shadow-sm'
                    }`}
                  >
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-bold text-slate-900 text-base">{t.name}</span>
                        <span
                          className={`w-2 h-2 rounded-full ${
                            t.online ? 'bg-emerald-500' : 'bg-slate-300'
                          }`}
                          title={t.online ? 'Онлайн' : 'Офлайн'}
                        />
                      </div>
                      <div className="text-xs text-slate-500 mt-0.5">
                        {t.members.join(', ')}
                      </div>
                      {isClosed && (
                        <div className={`text-xs font-semibold mt-2 ${fixedEventsCount > 0 ? 'text-indigo-600' : 'text-slate-400'}`}>
                          {fixedEventsCount > 0 ? `Событий выбрано: ${fixedEventsCount}` : 'События не выбраны (0)'}
                        </div>
                      )}
                    </div>

                    <div className="flex items-center gap-2">
                      {t.bankrupt ? (
                        <span className="px-2 py-0.5 bg-rose-100 text-rose-700 text-xs font-bold rounded-lg">
                          Банкрот
                        </span>
                      ) : t.submitted ? (
                        <span className="inline-flex items-center justify-center w-8 h-8 rounded-full bg-emerald-100 text-emerald-600 font-black text-sm">
                          ✓
                        </span>
                      ) : (
                        <span className="inline-flex items-center justify-center w-8 h-8 rounded-full bg-rose-100 text-rose-500 font-black text-sm">
                          ✕
                        </span>
                      )}
                    </div>
                  </button>
                );
              })}
            </div>
          </div>

          {/* Правая колонка: Детали команды и выбор событий (8 колонок) */}
          <div className="lg:col-span-8">
            {selectedTeam ? (
              <div className="bg-white border border-slate-200 rounded-3xl p-8 shadow-sm space-y-8">
                {/* Заголовок выбранной команды */}
                <div className="border-b border-slate-100 pb-6 flex items-start justify-between">
                  <div>
                    <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                      Информация о команде
                    </span>
                    <h2 className="text-2xl font-bold text-slate-900 mt-1">{selectedTeam.name}</h2>
                    <p className="text-sm text-slate-500 mt-1">{selectedTeam.members.join(', ')}</p>
                  </div>
                  <div className="text-right">
                    <span className={`inline-block px-3 py-1 rounded-full text-xs font-semibold ${
                      selectedTeam.submitted
                        ? 'bg-emerald-100 text-emerald-800'
                        : 'bg-rose-100 text-rose-800'
                    }`}>
                      {selectedTeam.submitted ? 'Решение сдано' : 'Решение не сдано'}
                    </span>
                  </div>
                </div>

                {/* 1. РАССЧИТАННЫЕ СОБЫТИЯ С ВЕРОЯТНОСТЯМИ (если раунд закрыт) */}
                {isClosed && (
                  <section className="space-y-4 bg-slate-50 border border-slate-200 rounded-3xl p-6">
                    <div className="flex items-start justify-between gap-4">
                      <div>
                        <h3 className="text-lg font-bold text-slate-900">
                          События для команды «{selectedTeam.name}»
                        </h3>
                        <p className="text-xs text-slate-500 mt-0.5">
                          Нажмите на карточку события для фиксации. Зафиксировано: {(eventSelections[selectedTeam.id] || new Set()).size}
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        <button
                          type="button"
                          onClick={() => selectSuggested(selectedTeam.id)}
                          className="text-xs font-semibold px-3 py-1.5 bg-slate-200 hover:bg-slate-300 text-slate-800 rounded-xl transition-colors"
                        >
                          Рекомендованные
                        </button>
                        <button
                          type="button"
                          onClick={() => clearEvents(selectedTeam.id)}
                          className="text-xs font-semibold px-3 py-1.5 bg-slate-100 hover:bg-slate-200 text-slate-600 rounded-xl transition-colors"
                        >
                          Очистить (0)
                        </button>
                      </div>
                    </div>

                    <div className="space-y-3 pt-1">
                      {(selectedTeam.predictions || []).length === 0 ? (
                        <p className="text-sm text-slate-400 italic py-4">Событий для этой команды не спрогнозировано.</p>
                      ) : (
                        selectedTeam.predictions.map((pred) => {
                          const isFixed = (eventSelections[selectedTeam.id] || new Set()).has(pred.event.id);
                          const pct = Math.round(pred.probability * 100);

                          return (
                            <button
                              key={pred.event.id}
                              type="button"
                              onClick={() => toggleEvent(selectedTeam.id, pred.event.id)}
                              className={`w-full text-left p-5 rounded-2xl border transition-all flex items-start justify-between gap-4 ${
                                isFixed
                                  ? 'bg-slate-900 text-white border-slate-900 shadow-md ring-2 ring-slate-900/20'
                                  : 'bg-white text-slate-900 border-slate-200 hover:border-slate-300 shadow-sm'
                              }`}
                            >
                              <div className="space-y-1 flex-1">
                                <div className="flex items-center gap-2">
                                  <span>{pred.event.isPositive ? '✅' : '⚠️'}</span>
                                  <span className="font-bold text-base">{pred.event.name}</span>
                                </div>
                                <p className={`text-xs leading-relaxed ${isFixed ? 'text-slate-300' : 'text-slate-500'}`}>
                                  {pred.event.description}
                                </p>
                              </div>

                              <div className="flex flex-col items-end gap-2 shrink-0">
                                <span className={`text-sm font-bold font-mono px-2 py-0.5 rounded-lg ${
                                  isFixed ? 'bg-slate-800 text-emerald-400' : 'bg-slate-100 text-slate-700'
                                }`}>
                                  {pct}% вер-ть
                                </span>
                                <span className={`text-xs font-semibold ${isFixed ? 'text-emerald-400' : 'text-slate-400'}`}>
                                  {isFixed ? 'Зафиксировано ✓' : 'Нажмите для выбора'}
                                </span>
                              </div>
                            </button>
                          );
                        })
                      )}
                    </div>
                  </section>
                )}

                {/* 2. РЕШЕНИЕ КОМАНДЫ НА ЭТОТ РАУНД */}
                {selectedTeam.decision ? (
                  <section className="space-y-6">
                    <h3 className="text-lg font-bold text-slate-900">Выбор команды на раунд {game.round}</h3>

                    {/* Атрибуты / Слайдеры */}
                    {selectedTeam.decision.attributes && selectedTeam.decision.attributes.length > 0 && (
                      <div className="space-y-2">
                        <span className="text-xs font-bold uppercase tracking-wider text-slate-400">
                          Распределение бюджета
                        </span>
                        <div className="grid grid-cols-2 gap-3">
                          {selectedTeam.decision.attributes.map((a) => {
                            const meta = selectedTeam.state?.configurable.find((c) => c.id === a.id);
                            return (
                              <div key={a.id} className="p-3 bg-slate-50 border border-slate-200 rounded-2xl flex items-center justify-between text-sm">
                                <span className="font-medium text-slate-700">{meta?.name || `Атрибут #${a.id}`}</span>
                                <span className="font-bold font-mono text-slate-900">{a.value.toFixed(2)}</span>
                              </div>
                            );
                          })}
                        </div>
                      </div>
                    )}

                    {/* Стратегии */}
                    {selectedTeam.decision.strategies && selectedTeam.decision.strategies.length > 0 && (
                      <div className="space-y-2">
                        <span className="text-xs font-bold uppercase tracking-wider text-slate-400">
                          Выбранные стратегии
                        </span>
                        <div className="flex flex-wrap gap-2">
                          {selectedTeam.decision.strategies.map((sid) => {
                            const strat = selectedTeam.state?.strategies.find((s) => s.id === sid);
                            return (
                              <span key={sid} className="px-3 py-1.5 bg-slate-900 text-white rounded-xl text-xs font-semibold">
                                {strat?.name || `Стратегия #${sid}`}
                              </span>
                            );
                          })}
                        </div>
                      </div>
                    )}

                    {/* Ответы на 4 вопроса отчета */}
                    <div className="space-y-4 pt-2">
                      <span className="text-xs font-bold uppercase tracking-wider text-slate-400">
                        Отчёт и рефлексия
                      </span>

                      <div className="space-y-3">
                        <ReportDetailCard
                          title="Почему именно такое решение?"
                          content={selectedTeam.decision.report?.why}
                        />
                        <ReportDetailCard
                          title="Какие риски с ним связаны?"
                          content={selectedTeam.decision.report?.risks}
                        />
                        <ReportDetailCard
                          title="Какие могут быть альтернативы?"
                          content={selectedTeam.decision.report?.alternative}
                        />
                        <ReportDetailCard
                          title="Если бы была возможность выбора вне игры, чтобы вы сделали?"
                          content={selectedTeam.decision.report?.correction}
                        />
                      </div>
                    </div>
                  </section>
                ) : (
                  <div className="py-12 text-center text-slate-400 text-sm">
                    {isOpen
                      ? 'Команда ещё не отправила решение за текущий раунд.'
                      : 'Решение за этот раунд не зафиксировано.'}
                  </div>
                )}
              </div>
            ) : (
              <div className="bg-white border border-slate-200 rounded-3xl p-12 text-center text-slate-400">
                Выберите команду из списка слева
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}

function ReportDetailCard({ title, content }: { title: string; content?: string }) {
  if (!content) return null;
  return (
    <div className="p-4 bg-slate-50 border border-slate-200 rounded-2xl space-y-1">
      <div className="text-xs font-bold text-slate-500">{title}</div>
      <div className="text-sm text-slate-800 whitespace-pre-wrap leading-relaxed">{content}</div>
    </div>
  );
}
