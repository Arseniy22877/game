import { useState } from 'react';
import { api, ApiRequestError } from '../../api';
import {
  TeamState,
  DecisionRequest,
  ConfigurableAttribute,
  StrategyItem,
  HintItem,
  GameEvent,
} from '../../types/api';

interface Props {
  state: TeamState;
  onUpdate: () => void;
}

/**
 * Основной игровой экран команды: бюджет, стратегии, исследования, отчёт.
 */
export function TeamGame({ state, onUpdate }: Props) {
  const { game, team, state: gameState, decision: existingDecision, opening } = state;
  const isRoundOpen = game.phase === 'round_open';
  const isRoundClosed = game.phase === 'round_closed';

  // Состояние формы решения
  const [attrValues, setAttrValues] = useState<Record<number, number>>(() => {
    const initial: Record<number, number> = {};
    if (existingDecision) {
      existingDecision.attributes.forEach((a) => { initial[a.id] = a.value; });
    } else if (gameState) {
      gameState.configurable.forEach((a) => { initial[a.id] = a.value; });
    }
    return initial;
  });

  const [selectedStrategies, setSelectedStrategies] = useState<Set<number>>(() => {
    if (existingDecision) return new Set(existingDecision.strategies);
    if (gameState) return new Set(gameState.strategies.filter((s) => s.active).map((s) => s.id));
    return new Set();
  });

  const [selectedHints, setSelectedHints] = useState<Set<number>>(() => {
    if (existingDecision) return new Set(existingDecision.hints);
    return new Set();
  });

  const [report, setReport] = useState({
    why: existingDecision?.report.why ?? '',
    alternative: existingDecision?.report.alternative ?? '',
    risks: existingDecision?.report.risks ?? '',
    correction: existingDecision?.report.correction ?? '',
  });

  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitSuccess, setSubmitSuccess] = useState(false);
  const [showOpening, setShowOpening] = useState(() => game.round === 1 && opening !== null && !existingDecision);
  const [showHistory, setShowHistory] = useState(false);
  const [showNewEvents, setShowNewEvents] = useState(() => {
    if (!gameState || game.round <= 1) return false;
    const lastRound = gameState.history.find((h) => h.round === game.round - 1);
    return lastRound !== undefined && lastRound.events.length > 0 && !existingDecision;
  });

  if (!gameState || !team) return null;

  // Расчёт стоимости
  const moneyCost =
    game.baseRoundCost +
    gameState.configurable.reduce((sum, attr) => {
      const val = attrValues[attr.id] ?? attr.value;
      return sum + Math.round(val * attr.costPerUnit);
    }, 0) +
    Array.from(selectedStrategies).reduce((sum, id) => {
      const s = gameState.strategies.find((s) => s.id === id);
      return sum + (s ? s.cost : 0);
    }, 0);

  const qpCost = Array.from(selectedHints).reduce((sum, id) => {
    const h = gameState.hints.find((h) => h.id === id);
    return sum + (h ? h.cost : 0);
  }, 0);

  const moneyOver = moneyCost > gameState.budget.money;
  const qpOver = qpCost > gameState.budget.qp;

  const toggleStrategy = (id: number) => {
    const next = new Set(selectedStrategies);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setSelectedStrategies(next);
  };

  const toggleHint = (id: number) => {
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
        value: attrValues[a.id] ?? a.value,
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
        setSubmitError('Произошла ошибка при отправке');
      }
    }
    setSubmitting(false);
  };

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Модальное окно вступительного события */}
      {showOpening && opening && (
        <EventModal
          title="Начало игры"
          events={[opening]}
          onClose={() => setShowOpening(false)}
        />
      )}

      {/* Модальное окно новых событий */}
      {showNewEvents && gameState.history.length > 0 && (
        <EventModal
          title={`События раунда ${game.round - 1}`}
          events={gameState.history.find((h) => h.round === game.round - 1)?.events ?? []}
          onClose={() => setShowNewEvents(false)}
        />
      )}

      {/* Модальное окно истории */}
      {showHistory && (
        <HistoryModal
          history={gameState.history}
          onClose={() => setShowHistory(false)}
        />
      )}

      {/* Шапка */}
      <header className="bg-white shadow-sm border-b">
        <div className="max-w-4xl mx-auto px-4 py-3 flex items-center justify-between">
          <div>
            <h1 className="text-lg font-bold">{team.name}</h1>
            <span className="text-sm text-gray-500">Раунд {game.round}</span>
          </div>
          <div className="flex gap-4 text-sm">
            <div className={`font-medium ${moneyOver ? 'text-red-600' : 'text-gray-700'}`}>
              💰 {gameState.budget.money - moneyCost} / {gameState.budget.money}
              {moneyOver && <span className="ml-1 text-red-500">⚠</span>}
            </div>
            <div className={`font-medium ${qpOver ? 'text-red-600' : 'text-gray-700'}`}>
              🔬 {gameState.budget.qp - qpCost} / {gameState.budget.qp}
              {qpOver && <span className="ml-1 text-red-500">⚠</span>}
            </div>
          </div>
        </div>
      </header>

      <main className="max-w-4xl mx-auto px-4 py-6 space-y-8">
        {/* Банкрот */}
        {team.bankrupt && (
          <div className="p-4 bg-red-50 border border-red-200 rounded-lg text-red-700">
            Недостаточно средств для участия в этом раунде. Ожидайте результатов.
          </div>
        )}

        {/* Раунд закрыт */}
        {isRoundClosed && (
          <div className="p-4 bg-yellow-50 border border-yellow-200 rounded-lg text-yellow-700">
            Раунд закрыт. Преподаватель выбирает события...
          </div>
        )}

        {/* Отправлено */}
        {(team.submitted || submitSuccess) && isRoundOpen && (
          <div className="p-4 bg-green-50 border border-green-200 rounded-lg text-green-700">
            ✓ Решение отправлено. Вы можете изменить его до закрытия раунда.
          </div>
        )}

        {/* Базовые атрибуты */}
        {gameState.attributes.length > 0 && (
          <section>
            <h2 className="text-lg font-semibold mb-3">Показатели</h2>
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
              {gameState.attributes.map((attr) => (
                <div key={attr.id} className="bg-white rounded-lg border p-3">
                  <div className="text-sm text-gray-500">{attr.name}</div>
                  <div className="text-xl font-bold">{attr.value.toFixed(2)}</div>
                  <div className="mt-1 w-full bg-gray-200 rounded-full h-2">
                    <div
                      className="bg-blue-500 h-2 rounded-full"
                      style={{ width: `${((attr.value - attr.min) / (attr.max - attr.min)) * 100}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </section>
        )}

        {/* Сферы бюджета (слайдеры) */}
        {gameState.configurable.length > 0 && (
          <section>
            <h2 className="text-lg font-semibold mb-3">Распределение бюджета</h2>
            <div className="space-y-4">
              {gameState.configurable.map((attr) => (
                <SliderRow
                  key={attr.id}
                  attr={attr}
                  value={attrValues[attr.id] ?? attr.value}
                  onChange={(v) => setAttrValues({ ...attrValues, [attr.id]: v })}
                  disabled={!isRoundOpen || team.bankrupt}
                />
              ))}
            </div>
          </section>
        )}

        {/* Стратегии */}
        {gameState.strategies.length > 0 && (
          <section>
            <h2 className="text-lg font-semibold mb-3">Стратегии</h2>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {gameState.strategies.map((s) => (
                <StrategyCard
                  key={s.id}
                  strategy={s}
                  selected={selectedStrategies.has(s.id)}
                  onToggle={() => toggleStrategy(s.id)}
                  disabled={!isRoundOpen || team.bankrupt}
                />
              ))}
            </div>
          </section>
        )}

        {/* Исследования */}
        {gameState.hints.length > 0 && (
          <section>
            <h2 className="text-lg font-semibold mb-3">Исследования</h2>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {gameState.hints.map((h) => (
                <HintCard
                  key={h.id}
                  hint={h}
                  selected={selectedHints.has(h.id)}
                  onToggle={() => toggleHint(h.id)}
                  disabled={!isRoundOpen || team.bankrupt || h.used}
                />
              ))}
            </div>
          </section>
        )}

        {/* Отчёт */}
        {isRoundOpen && !team.bankrupt && (
          <section>
            <h2 className="text-lg font-semibold mb-3">Отчёт</h2>
            <div className="space-y-4">
              <ReportField
                label="Почему именно такое решение?"
                value={report.why}
                onChange={(v) => setReport({ ...report, why: v })}
              />
              <ReportField
                label="Какое могло быть другое решение?"
                value={report.alternative}
                onChange={(v) => setReport({ ...report, alternative: v })}
              />
              <ReportField
                label="Какие риски связаны с вашим решением?"
                value={report.risks}
                onChange={(v) => setReport({ ...report, risks: v })}
              />
              <ReportField
                label="Что изменили бы, если бы это была реальная жизнь?"
                value={report.correction}
                onChange={(v) => setReport({ ...report, correction: v })}
              />
            </div>
          </section>
        )}

        {/* Ошибка отправки */}
        {submitError && (
          <div className="p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">
            {submitError}
          </div>
        )}

        {/* Кнопки */}
        <div className="flex gap-3">
          <button
            onClick={() => setShowHistory(true)}
            className="px-4 py-2 border border-gray-300 rounded-lg text-gray-700 hover:bg-gray-100"
          >
            История событий
          </button>
          {isRoundOpen && !team.bankrupt && (
            <button
              onClick={handleSubmit}
              disabled={submitting || moneyOver || qpOver}
              className="flex-1 py-3 bg-blue-600 text-white font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {submitting
                ? 'Отправка...'
                : team.submitted
                  ? 'Переотправить решение'
                  : 'Завершить раунд'}
            </button>
          )}
        </div>
      </main>
    </div>
  );
}

// --- Вспомогательные компоненты ---

function SliderRow({
  attr,
  value,
  onChange,
  disabled,
}: {
  attr: ConfigurableAttribute;
  value: number;
  onChange: (v: number) => void;
  disabled: boolean;
}) {
  const cost = Math.round(value * attr.costPerUnit);
  return (
    <div className="bg-white rounded-lg border p-4">
      <div className="flex justify-between items-center mb-2">
        <div>
          <span className="font-medium">{attr.name}</span>
          <span className="text-sm text-gray-500 ml-2">{attr.description}</span>
        </div>
        <div className="text-sm font-medium text-gray-600">
          💰 {cost}
        </div>
      </div>
      <div className="flex items-center gap-3">
        <span className="text-xs text-gray-400">{attr.min}</span>
        <input
          type="range"
          min={attr.min}
          max={attr.max}
          step={0.01}
          value={value}
          onChange={(e) => onChange(parseFloat(e.target.value))}
          disabled={disabled}
          className="flex-1"
        />
        <span className="text-xs text-gray-400">{attr.max}</span>
        <span className="text-sm font-mono w-12 text-right">{value.toFixed(2)}</span>
      </div>
    </div>
  );
}

function StrategyCard({
  strategy,
  selected,
  onToggle,
  disabled,
}: {
  strategy: StrategyItem;
  selected: boolean;
  onToggle: () => void;
  disabled: boolean;
}) {
  return (
    <button
      onClick={onToggle}
      disabled={disabled}
      className={`text-left p-4 rounded-lg border-2 transition-colors ${
        selected
          ? 'border-blue-500 bg-blue-50'
          : 'border-gray-200 bg-white hover:border-gray-300'
      } disabled:opacity-50 disabled:cursor-not-allowed`}
    >
      <div className="flex justify-between items-start">
        <span className="font-medium">{strategy.name}</span>
        <span className="text-sm text-gray-500">💰 {strategy.cost}/раунд</span>
      </div>
      <p className="text-sm text-gray-600 mt-1">{strategy.description}</p>
      {strategy.active && !selected && (
        <span className="inline-block mt-2 text-xs bg-yellow-100 text-yellow-700 px-2 py-0.5 rounded">
          Активна (будет отключена)
        </span>
      )}
    </button>
  );
}

function HintCard({
  hint,
  selected,
  onToggle,
  disabled,
}: {
  hint: HintItem;
  selected: boolean;
  onToggle: () => void;
  disabled: boolean;
}) {
  return (
    <div
      className={`p-4 rounded-lg border-2 ${
        hint.used
          ? 'border-green-200 bg-green-50'
          : selected
            ? 'border-blue-500 bg-blue-50'
            : 'border-gray-200 bg-white'
      }`}
    >
      <div className="flex justify-between items-start">
        <span className="font-medium">{hint.name}</span>
        <span className="text-sm text-gray-500">🔬 {hint.cost} QP</span>
      </div>
      <p className="text-sm text-gray-600 mt-1">{hint.description}</p>
      <div className="text-xs text-gray-400 mt-1">Надёжность: {hint.reliability}</div>

      {hint.used && hint.secret && (
        <div className="mt-3 p-3 bg-white rounded border border-green-300 text-sm">
          <div className="text-xs text-green-600 font-medium mb-1">Результат:</div>
          {hint.secret}
        </div>
      )}

      {!hint.used && (
        <button
          onClick={onToggle}
          disabled={disabled}
          className={`mt-3 w-full py-2 text-sm font-medium rounded-lg transition-colors ${
            selected
              ? 'bg-blue-600 text-white'
              : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
          } disabled:opacity-50 disabled:cursor-not-allowed`}
        >
          {selected ? 'Выбрано ✓' : 'Купить'}
        </button>
      )}
    </div>
  );
}

function ReportField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <div>
      <label className="block text-sm font-medium text-gray-700 mb-1">{label}</label>
      <textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        maxLength={4000}
        rows={3}
        className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none resize-y"
      />
    </div>
  );
}

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
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="bg-white rounded-2xl shadow-xl max-w-lg w-full max-h-[80vh] overflow-y-auto p-6">
        <h2 className="text-xl font-bold mb-4">{title}</h2>
        {events.length === 0 ? (
          <p className="text-gray-500">Событий не произошло.</p>
        ) : (
          <div className="space-y-4">
            {events.map((ev) => (
              <div
                key={ev.id}
                className={`p-4 rounded-lg border-2 ${
                  ev.isPositive ? 'border-green-200 bg-green-50' : 'border-red-200 bg-red-50'
                }`}
              >
                {ev.imageUrl && (
                  <img
                    src={ev.imageUrl}
                    alt={ev.name}
                    className="w-full h-40 object-cover rounded-lg mb-3"
                  />
                )}
                <div className="flex items-center gap-2 mb-1">
                  <span>{ev.isPositive ? '✅' : '⚠️'}</span>
                  <span className="font-semibold">{ev.name}</span>
                </div>
                <p className="text-sm text-gray-700">{ev.description}</p>
              </div>
            ))}
          </div>
        )}
        <button
          onClick={onClose}
          className="mt-6 w-full py-2 bg-gray-800 text-white rounded-lg hover:bg-gray-900"
        >
          Понятно
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
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="bg-white rounded-2xl shadow-xl max-w-lg w-full max-h-[80vh] overflow-y-auto p-6">
        <h2 className="text-xl font-bold mb-4">История событий</h2>
        {history.length === 0 ? (
          <p className="text-gray-500">Пока нет событий.</p>
        ) : (
          <div className="space-y-6">
            {history.map((round) => (
              <div key={round.round}>
                <h3 className="font-medium text-gray-800 mb-2">Раунд {round.round}</h3>
                {round.events.length === 0 ? (
                  <p className="text-sm text-gray-400">Без событий</p>
                ) : (
                  <div className="space-y-2">
                    {round.events.map((ev) => (
                      <div
                        key={ev.id}
                        className={`p-3 rounded-lg text-sm ${
                          ev.isPositive ? 'bg-green-50' : 'bg-red-50'
                        }`}
                      >
                        <span>{ev.isPositive ? '✅' : '⚠️'}</span>{' '}
                        <span className="font-medium">{ev.name}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
        <button
          onClick={onClose}
          className="mt-6 w-full py-2 bg-gray-800 text-white rounded-lg hover:bg-gray-900"
        >
          Закрыть
        </button>
      </div>
    </div>
  );
}

