import { useState } from 'react';
import { api, ApiRequestError } from '../../api';
import {
  TeamState,
  DecisionRequest,
  HintItem,
  GameEvent,
} from '../../types/api';

interface Props {
  state: TeamState;
  onUpdate: () => void;
}

/**
 * Экран 3: Игровой стол команды (Основная страница игры).
 * В точности воспроизводит страницу 2 PDF:
 * - Бюджетирование (слайдеры по сферам с суммами в $)
 * - Стратегии (карточки с описанием и стоимостью, темное выделение при выборе)
 * - Исследования (карточки со статусами и модальным окном информации)
 * - Отчет из 4 вопросов
 * - Сайдбар: Раунд, Бюджет, Очки исследования, Показатели, История событий, Завершить раунд
 */
export function TeamGame({ state, onUpdate }: Props) {
  const { game, team, state: gameState, decision: existingDecision, opening } = state;
  const isRoundOpen = game.phase === 'round_open';
  const isRoundClosed = game.phase === 'round_closed';

  // Инициализация значений формы
  const [attrValues, setAttrValues] = useState<Record<number, number>>(() => {
    const initial: Record<number, number> = {};
    if (existingDecision && existingDecision.attributes) {
      existingDecision.attributes.forEach((a) => { initial[a.id] = a.value; });
    } else if (gameState) {
      gameState.configurable.forEach((a) => { initial[a.id] = a.value; });
    }
    return initial;
  });

  const [selectedStrategies, setSelectedStrategies] = useState<Set<number>>(() => {
    if (existingDecision && existingDecision.strategies) return new Set(existingDecision.strategies);
    if (gameState) return new Set(gameState.strategies.filter((s) => s.active).map((s) => s.id));
    return new Set();
  });

  const [selectedHints, setSelectedHints] = useState<Set<number>>(() => {
    if (existingDecision && existingDecision.hints) return new Set(existingDecision.hints);
    return new Set();
  });

  const [report, setReport] = useState({
    why: existingDecision?.report?.why ?? '',
    alternative: existingDecision?.report?.alternative ?? '',
    risks: existingDecision?.report?.risks ?? '',
    correction: existingDecision?.report?.correction ?? '',
  });

  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitSuccess, setSubmitSuccess] = useState(false);

  // Модальные окна
  const [showOpening, setShowOpening] = useState(() => game.round === 1 && opening !== null && !existingDecision);
  const [showRoundEvents, setShowRoundEvents] = useState(() => {
    if (game.round <= 1 || !gameState) return false;
    const prevHistory = gameState.history.find((h) => h.round === game.round - 1);
    return !!prevHistory && prevHistory.events.length > 0 && !existingDecision;
  });
  const [showHistory, setShowHistory] = useState(false);
  const [activeHintModal, setActiveHintModal] = useState<HintItem | null>(null);

  if (!gameState || !team) return null;

  // Расчёт текущих затрат
  const moneySpent =
    game.baseRoundCost +
    gameState.configurable.reduce((sum, attr) => {
      const val = attrValues[attr.id] !== undefined ? attrValues[attr.id] : attr.value;
      return sum + Math.round(val * attr.costPerUnit);
    }, 0) +
    Array.from(selectedStrategies).reduce((sum, id) => {
      const s = gameState.strategies.find((strat) => strat.id === id);
      return sum + (s ? s.cost : 0);
    }, 0);

  const qpSpent = Array.from(selectedHints).reduce((sum, id) => {
    const h = gameState.hints.find((hint) => hint.id === id);
    return sum + (h ? h.cost : 0);
  }, 0);

  const remainingMoney = gameState.budget.money - moneySpent;
  const remainingQP = gameState.budget.qp - qpSpent;

  const moneyOver = remainingMoney < 0;
  const qpOver = remainingQP < 0;

  const toggleStrategy = (id: number) => {
    if (!isRoundOpen || team.bankrupt) return;
    const next = new Set(selectedStrategies);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setSelectedStrategies(next);
  };

  const toggleHint = (id: number) => {
    if (!isRoundOpen || team.bankrupt) return;
    const hint = gameState.hints.find((h) => h.id === id);
    if (!hint || hint.used) return;

    const next = new Set(selectedHints);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setSelectedHints(next);
  };

  const handleSubmit = async () => {
    setSubmitError(null);
    setSubmitSuccess(false);

    const decision: DecisionRequest = {
      round: game.round,
      attributes: gameState.configurable.map((a) => ({
        id: a.id,
        value: attrValues[a.id] !== undefined ? attrValues[a.id] : a.value,
      })),
      strategies: Array.from(selectedStrategies),
      hints: Array.from(selectedHints),
      report,
    };

    setSubmitting(true);
    try {
      await api.submitDecision(decision);
      setSubmitSuccess(true);
      onUpdate();
    } catch (e: unknown) {
      if (e instanceof ApiRequestError) {
        setSubmitError(e.userMessage);
      } else if (e instanceof Error) {
        setSubmitError(e.message);
      } else {
        setSubmitError('Произошла ошибка при отправке решения');
      }
    }
    setSubmitting(false);
  };

  return (
    <div className="min-h-screen bg-slate-50 flex flex-col">
      {/* Модальное окно вступительного события */}
      {showOpening && opening && (
        <EventModal
          title={opening.name || 'Вводный сценарий'}
          events={[opening]}
          onClose={() => setShowOpening(false)}
        />
      )}

      {/* Модальное окно событий нового раунда */}
      {showRoundEvents && gameState && (
        <EventModal
          title={`Произошедшие события (раунд ${game.round - 1})`}
          events={gameState.history.find((h) => h.round === game.round - 1)?.events || []}
          onClose={() => setShowRoundEvents(false)}
        />
      )}

      {/* Модальное окно истории */}
      {showHistory && (
        <HistoryModal
          history={gameState.history}
          onClose={() => setShowHistory(false)}
        />
      )}

      {/* Модальное окно завершенного исследования */}
      {activeHintModal && (
        <HintDetailModal
          hint={activeHintModal}
          onClose={() => setActiveHintModal(null)}
        />
      )}

      {/* Шапка */}
      <header className="bg-white border-b border-slate-200 sticky top-0 z-20">
        <div className="max-w-7xl mx-auto px-4 py-3.5 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">Команда</span>
            <span className="text-lg font-bold text-slate-900">{team.name}</span>
            <span className="text-xs font-medium text-slate-500">
              ({team.members.join(', ')})
            </span>
          </div>

          <div className="flex items-center gap-3">
            {team.bankrupt && (
              <span className="px-3 py-1 bg-rose-50 text-rose-600 border border-rose-200 text-xs font-semibold rounded-full">
                Банкрот в этом раунде
              </span>
            )}
            {isRoundClosed && (
              <span className="px-3 py-1 bg-amber-50 text-amber-700 border border-amber-200 text-xs font-semibold rounded-full animate-pulse">
                Раунд закрыт преподавателем
              </span>
            )}
            {team.submitted && isRoundOpen && (
              <span className="px-3 py-1 bg-emerald-50 text-emerald-700 border border-emerald-200 text-xs font-semibold rounded-full">
                ✓ Решение отправлено
              </span>
            )}
          </div>
        </div>
      </header>

      {/* Основной контент (Двухколоночный макет по PDF) */}
      <main className="max-w-7xl mx-auto px-4 py-8 flex-1 w-full">
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 items-start">
          {/* Левая колонка: Основные блоки игры (8 колонок) */}
          <div className="lg:col-span-8 space-y-10">
            {/* Ошибки или успешные оповещения */}
            {submitError && (
              <div className="p-4 bg-rose-50 border border-rose-200 rounded-2xl text-rose-700 text-sm font-medium">
                {submitError}
              </div>
            )}
            {submitSuccess && (
              <div className="p-4 bg-emerald-50 border border-emerald-200 rounded-2xl text-emerald-800 text-sm font-medium flex items-center justify-between">
                <span>✓ Ваше решение успешно зафиксировано. Вы можете изменить его до закрытия раунда.</span>
                <button onClick={() => setSubmitSuccess(false)} className="text-emerald-600 hover:text-emerald-800 font-bold">✕</button>
              </div>
            )}

            {/* 1. БЮДЖЕТИРОВАНИЕ */}
            <section className="space-y-4">
              <h2 className="text-xl font-bold text-slate-900 tracking-tight">Бюджетирование</h2>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {gameState.configurable.map((attr) => {
                  const val = attrValues[attr.id] !== undefined ? attrValues[attr.id] : attr.value;
                  const cost = Math.round(val * attr.costPerUnit);
                  return (
                    <div
                      key={attr.id}
                      className="bg-white border border-slate-200 rounded-2xl p-5 shadow-sm space-y-3"
                    >
                      <div className="flex items-center justify-between">
                        <span className="font-semibold text-slate-800 text-sm">{attr.name}</span>
                        <span className="text-emerald-600 font-bold text-base font-mono">
                          {cost}K $
                        </span>
                      </div>
                      <div className="flex items-center gap-3 pt-1">
                        <input
                          type="range"
                          min={attr.min}
                          max={attr.max}
                          step={0.01}
                          value={val}
                          disabled={!isRoundOpen || team.bankrupt}
                          onChange={(e) => setAttrValues({ ...attrValues, [attr.id]: parseFloat(e.target.value) })}
                          className="flex-1 accent-slate-900 cursor-pointer disabled:cursor-not-allowed"
                        />
                      </div>
                      {attr.description && (
                        <p className="text-xs text-slate-400 line-clamp-1">{attr.description}</p>
                      )}
                    </div>
                  );
                })}
              </div>
            </section>

            {/* 2. СТРАТЕГИИ */}
            <section className="space-y-4">
              <div className="flex items-center justify-between">
                <h2 className="text-xl font-bold text-slate-900 tracking-tight">Стратегии</h2>
                <span className="text-xs text-slate-400">Нажмите на карточку для выбора</span>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {gameState.strategies.map((strategy) => {
                  const isSelected = selectedStrategies.has(strategy.id);
                  return (
                    <button
                      key={strategy.id}
                      type="button"
                      disabled={!isRoundOpen || team.bankrupt}
                      onClick={() => toggleStrategy(strategy.id)}
                      className={`text-left p-6 rounded-3xl border transition-all flex flex-col justify-between space-y-4 ${
                        isSelected
                          ? 'bg-slate-900 text-white border-slate-900 shadow-md ring-2 ring-slate-900/20'
                          : 'bg-white text-slate-900 border-slate-200 hover:border-slate-300 shadow-sm'
                      } disabled:opacity-60 disabled:cursor-not-allowed`}
                    >
                      <div>
                        <div className="flex items-start justify-between gap-2 mb-2">
                          <h3 className={`font-bold text-base ${isSelected ? 'text-white' : 'text-slate-900'}`}>
                            {strategy.name}
                          </h3>
                        </div>
                        <p className={`text-xs leading-relaxed ${isSelected ? 'text-slate-300' : 'text-slate-500'}`}>
                          {strategy.description}
                        </p>
                      </div>

                      <div className="flex items-center justify-between pt-2 border-t border-slate-100/10">
                        <span className={`text-sm font-bold font-mono ${isSelected ? 'text-emerald-400' : 'text-emerald-600'}`}>
                          {strategy.cost}K $
                        </span>
                        {isSelected && (
                          <span className="text-xs font-semibold text-slate-300 flex items-center gap-1">
                            ✓ Стратегия выбрана
                          </span>
                        )}
                      </div>
                    </button>
                  );
                })}
              </div>
            </section>

            {/* 3. ИССЛЕДОВАНИЯ */}
            <section className="space-y-4">
              <div className="flex items-center justify-between">
                <h2 className="text-xl font-bold text-slate-900 tracking-tight">Исследования</h2>
                <span className="text-xs text-slate-400">Оплачиваются за QP</span>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {gameState.hints.map((hint) => {
                  const isSelected = selectedHints.has(hint.id);
                  const isCompleted = hint.used;

                  if (isCompleted) {
                    return (
                      <div
                        key={hint.id}
                        onClick={() => setActiveHintModal(hint)}
                        className="bg-slate-900 text-white rounded-3xl p-6 shadow-sm border border-slate-900 cursor-pointer hover:bg-slate-800 transition-all flex flex-col justify-between space-y-3"
                      >
                        <div>
                          <div className="flex items-center justify-between mb-2">
                            <h3 className="font-bold text-base text-white">{hint.name}</h3>
                            <span className="text-emerald-400 font-bold text-sm">✓</span>
                          </div>
                          <p className="text-xs text-slate-300 line-clamp-2">{hint.description}</p>
                        </div>
                        <div className="pt-2 border-t border-slate-800 flex items-center justify-between text-xs text-emerald-400 font-medium">
                          <span>Исследование завершено</span>
                          <span className="underline">Открыть информацию →</span>
                        </div>
                      </div>
                    );
                  }

                  return (
                    <button
                      key={hint.id}
                      type="button"
                      disabled={!isRoundOpen || team.bankrupt}
                      onClick={() => toggleHint(hint.id)}
                      className={`text-left p-6 rounded-3xl border transition-all flex flex-col justify-between space-y-4 ${
                        isSelected
                          ? 'bg-slate-900 text-white border-slate-900 shadow-md ring-2 ring-slate-900/20'
                          : 'bg-white text-slate-900 border-slate-200 hover:border-slate-300 shadow-sm'
                      } disabled:opacity-60 disabled:cursor-not-allowed`}
                    >
                      <div>
                        <h3 className={`font-bold text-base mb-2 ${isSelected ? 'text-white' : 'text-slate-900'}`}>
                          {hint.name}
                        </h3>
                        <p className={`text-xs leading-relaxed ${isSelected ? 'text-slate-300' : 'text-slate-500'}`}>
                          {hint.description}
                        </p>
                      </div>

                      <div className="flex items-center justify-between pt-2 border-t border-slate-100/10">
                        <span className={`text-sm font-bold font-mono ${isSelected ? 'text-cyan-400' : 'text-cyan-600'}`}>
                          {hint.cost} QP
                        </span>
                        {isSelected ? (
                          <span className="text-xs font-semibold text-slate-300">
                            Выбрано на след. ход ✓
                          </span>
                        ) : (
                          <span className="text-xs font-medium text-slate-400">
                            Нажмите для выбора
                          </span>
                        )}
                      </div>
                    </button>
                  );
                })}
              </div>
            </section>

            {/* 4. ОТЧЁТ (4 поля из PDF) */}
            <section className="bg-white border border-slate-200 rounded-3xl p-6 shadow-sm space-y-6">
              <h2 className="text-xl font-bold text-slate-900 tracking-tight">Отчёт</h2>

              <div className="space-y-5">
                <div>
                  <label className="block text-xs font-semibold text-slate-600 uppercase tracking-wider mb-2">
                    Почему именно такое решение?
                  </label>
                  <textarea
                    value={report.why}
                    disabled={!isRoundOpen || team.bankrupt}
                    onChange={(e) => setReport({ ...report, why: e.target.value })}
                    rows={3}
                    placeholder="Опишите аргументацию выбранных параметров бюджета и стратегий..."
                    className="w-full px-4 py-3 bg-slate-50 border border-slate-200 rounded-2xl text-slate-900 text-sm focus:bg-white focus:ring-2 focus:ring-slate-900 outline-none transition-all resize-y disabled:opacity-60"
                  />
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-600 uppercase tracking-wider mb-2">
                    Какие риски с ним связаны?
                  </label>
                  <textarea
                    value={report.risks}
                    disabled={!isRoundOpen || team.bankrupt}
                    onChange={(e) => setReport({ ...report, risks: e.target.value })}
                    rows={3}
                    placeholder="Оцените возможные негативные события и уязвимости решения..."
                    className="w-full px-4 py-3 bg-slate-50 border border-slate-200 rounded-2xl text-slate-900 text-sm focus:bg-white focus:ring-2 focus:ring-slate-900 outline-none transition-all resize-y disabled:opacity-60"
                  />
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-600 uppercase tracking-wider mb-2">
                    Какие могут быть альтернативы?
                  </label>
                  <textarea
                    value={report.alternative}
                    disabled={!isRoundOpen || team.bankrupt}
                    onChange={(e) => setReport({ ...report, alternative: e.target.value })}
                    rows={3}
                    placeholder="Какие альтернативные варианты рассматривала команда?.."
                    className="w-full px-4 py-3 bg-slate-50 border border-slate-200 rounded-2xl text-slate-900 text-sm focus:bg-white focus:ring-2 focus:ring-slate-900 outline-none transition-all resize-y disabled:opacity-60"
                  />
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-600 uppercase tracking-wider mb-2">
                    Если бы была возможность выбора вне игры, чтобы вы сделали?
                  </label>
                  <textarea
                    value={report.correction}
                    disabled={!isRoundOpen || team.bankrupt}
                    onChange={(e) => setReport({ ...report, correction: e.target.value })}
                    rows={3}
                    placeholder="Действия в реальном мире за рамками предложенных механик..."
                    className="w-full px-4 py-3 bg-slate-50 border border-slate-200 rounded-2xl text-slate-900 text-sm focus:bg-white focus:ring-2 focus:ring-slate-900 outline-none transition-all resize-y disabled:opacity-60"
                  />
                </div>
              </div>
            </section>
          </div>

          {/* Правая колонка: Сайдбар статуса (4 колонки) */}
          <div className="lg:col-span-4 space-y-6 sticky top-24">
            <div className="bg-white border border-slate-200 rounded-3xl p-6 shadow-sm space-y-6">
              {/* Номер раунда */}
              <div className="border-b border-slate-100 pb-4">
                <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">Текущий этап</span>
                <h2 className="text-2xl font-black text-slate-900 mt-1">Раунд {game.round}</h2>
              </div>

              {/* Бюджет */}
              <div className="space-y-1">
                <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">Бюджет</span>
                <div className={`text-2xl font-black font-mono ${moneyOver ? 'text-rose-600' : 'text-emerald-600'}`}>
                  {remainingMoney} / {gameState.budget.money} K $
                </div>
                <div className="text-xs text-slate-400">
                  Затраты на раунд: {moneySpent} K $
                </div>
                {moneyOver && (
                  <div className="text-xs font-semibold text-rose-600 mt-2 flex items-center gap-1.5 bg-rose-50 p-2.5 rounded-xl border border-rose-200">
                    <svg className="w-4 h-4 shrink-0 text-rose-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                    <span>Превышен лимит доступных средств</span>
                  </div>
                )}
              </div>

              {/* Очки исследования */}
              <div className="space-y-1 border-t border-slate-100 pt-4">
                <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">Очки исследования</span>
                <div className={`text-2xl font-black font-mono ${qpOver ? 'text-rose-600' : 'text-cyan-600'}`}>
                  {remainingQP} / {gameState.budget.qp} QP
                </div>
                {qpOver && (
                  <div className="text-xs font-semibold text-rose-600 mt-2 flex items-center gap-1.5 bg-rose-50 p-2.5 rounded-xl border border-rose-200">
                    <svg className="w-4 h-4 shrink-0 text-rose-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                    <span>Недостаточно QP на исследования</span>
                  </div>
                )}
              </div>

              {/* Показатели */}
              <div className="border-t border-slate-100 pt-4 space-y-3">
                <span className="text-xs font-semibold uppercase tracking-wider text-slate-400 block">Показатели компании</span>
                <div className="space-y-2.5">
                  {gameState.attributes.map((attr) => (
                    <div
                      key={attr.id}
                      className="flex items-center justify-between p-3 bg-slate-50 border border-slate-200 rounded-2xl"
                    >
                      <span className="text-xs font-semibold text-slate-700">{attr.name}</span>
                      <span className="text-sm font-bold font-mono text-slate-900 bg-white px-2.5 py-1 rounded-xl border border-slate-200 shadow-2xs">
                        {attr.max <= 1 && attr.min >= 0 ? `${Math.round(attr.value * 100)}%` : Math.round(attr.value)}
                      </span>
                    </div>
                  ))}
                </div>
              </div>

              {/* Кнопка "История событий" */}
              <div className="border-t border-slate-100 pt-4">
                <button
                  type="button"
                  onClick={() => setShowHistory(true)}
                  className="w-full py-3 bg-slate-100 hover:bg-slate-200 text-slate-800 text-sm font-bold rounded-2xl transition-colors shadow-2xs"
                >
                  История событий
                </button>
              </div>

              {/* Кнопка "Завершить раунд" */}
              {isRoundOpen && !team.bankrupt && (
                <button
                  type="button"
                  disabled={submitting || moneyOver || qpOver}
                  onClick={handleSubmit}
                  className="w-full py-4 bg-slate-900 hover:bg-slate-800 text-white text-base font-bold rounded-2xl transition-all shadow-sm disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {submitting
                    ? 'Отправка решения...'
                    : team.submitted
                      ? 'Переотправить решение'
                      : 'Завершить раунд'}
                </button>
              )}
            </div>
          </div>
        </div>
      </main>
    </div>
  );
}

// --- Модальные окна ---

function EventModal({
  title,
  events,
  onClose,
}: {
  title: string;
  events: GameEvent[];
  onClose: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60 backdrop-blur-xs p-4">
      <div className="bg-white rounded-3xl shadow-xl max-w-lg w-full max-h-[85vh] overflow-y-auto p-7 space-y-5 animate-in fade-in duration-200">
        <h2 className="text-2xl font-bold text-slate-900">{title}</h2>
        <div className="space-y-4">
          {events.map((ev) => (
            <div
              key={ev.id}
              className={`p-5 rounded-2xl border ${
                ev.isPositive
                  ? 'border-emerald-200 bg-emerald-50/50 text-emerald-950'
                  : 'border-rose-200 bg-rose-50/50 text-rose-950'
              }`}
            >
              <div className="flex items-center gap-2.5 mb-2">
                <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-semibold ${
                  ev.isPositive
                    ? 'bg-emerald-100 text-emerald-800 border border-emerald-200'
                    : 'bg-rose-100 text-rose-800 border border-rose-200'
                }`}>
                  <span className={`w-1.5 h-1.5 rounded-full ${ev.isPositive ? 'bg-emerald-500' : 'bg-rose-500'}`} />
                  {ev.isPositive ? 'Позитивное' : 'Риск'}
                </span>
                <span className="font-bold text-base text-slate-900">{ev.name}</span>
              </div>
              <p className="text-sm leading-relaxed text-slate-700">{ev.description}</p>
            </div>
          ))}
        </div>
        <button
          type="button"
          onClick={onClose}
          className="w-full py-3 bg-slate-900 text-white font-bold rounded-2xl hover:bg-slate-800 transition-colors"
        >
          Продолжить
        </button>
      </div>
    </div>
  );
}

function HistoryModal({
  history,
  onClose,
}: {
  history: { round: number; events: GameEvent[] }[];
  onClose: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60 backdrop-blur-xs p-4">
      <div className="bg-white rounded-3xl shadow-xl max-w-lg w-full max-h-[85vh] overflow-y-auto p-7 space-y-6">
        <div className="flex items-center justify-between">
          <h2 className="text-2xl font-bold text-slate-900">История событий</h2>
          <button onClick={onClose} className="text-slate-400 hover:text-slate-600 font-bold text-lg">✕</button>
        </div>

        {history.length === 0 ? (
          <p className="text-sm text-slate-400 py-8 text-center">Событий пока не произошло.</p>
        ) : (
          <div className="space-y-6">
            {history.map((r) => (
              <div key={r.round} className="space-y-2">
                <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400">
                  {r.round === 0 ? 'Начало игры' : `Раунд ${r.round}`}
                </h3>
                {r.events.length === 0 ? (
                  <div className="text-xs text-slate-400 italic">Событий без происшествий</div>
                ) : (
                  r.events.map((ev) => (
                    <div
                      key={ev.id}
                      className={`p-4 rounded-2xl border text-sm ${
                        ev.isPositive
                          ? 'border-emerald-200 bg-emerald-50 text-emerald-900'
                          : 'border-rose-200 bg-rose-50 text-rose-900'
                      }`}
                    >
                      <span className="font-bold">{ev.name}: </span>
                      <span className="text-slate-700">{ev.description}</span>
                    </div>
                  ))
                )}
              </div>
            ))}
          </div>
        )}

        <button
          type="button"
          onClick={onClose}
          className="w-full py-3 bg-slate-900 text-white font-bold rounded-2xl hover:bg-slate-800 transition-colors"
        >
          Закрыть
        </button>
      </div>
    </div>
  );
}

function HintDetailModal({
  hint,
  onClose,
}: {
  hint: HintItem;
  onClose: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60 backdrop-blur-xs p-4">
      <div className="bg-white rounded-3xl shadow-xl max-w-md w-full p-7 space-y-5">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-bold text-slate-900">{hint.name}</h2>
          <button onClick={onClose} className="text-slate-400 hover:text-slate-600 font-bold">✕</button>
        </div>

        <p className="text-xs text-slate-500 leading-relaxed">{hint.description}</p>

        <div className="p-5 bg-emerald-50 border border-emerald-200 rounded-2xl space-y-2">
          <div className="text-xs font-bold uppercase tracking-wider text-emerald-700">
            Результаты исследования:
          </div>
          <div className="text-sm font-medium text-emerald-950 leading-relaxed">
            {hint.secret || 'Информация получена.'}
          </div>
        </div>

        <button
          type="button"
          onClick={onClose}
          className="w-full py-3 bg-slate-900 text-white font-bold rounded-2xl hover:bg-slate-800 transition-colors"
        >
          Понятно
        </button>
      </div>
    </div>
  );
}
