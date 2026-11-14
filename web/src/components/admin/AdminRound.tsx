import { useState } from 'react';
import { api } from '../../api';
import { AdminState, AdminTeam, Prediction } from '../../types/api';

interface Props {
  state: AdminState;
  onUpdate: () => void;
}

/**
 * Экран раунда у преподавателя (round_open / round_closed).
 */
export function AdminRound({ state, onUpdate }: Props) {
  const { game, teams } = state;
  const isOpen = game.phase === 'round_open';
  const isClosed = game.phase === 'round_closed';

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedTeamId, setSelectedTeamId] = useState<number | null>(null);

  // Выбранные события для каждой команды (только в round_closed)
  const [eventSelections, setEventSelections] = useState<Record<number, Set<number>>>(() => {
    const init: Record<number, Set<number>> = {};
    teams.forEach((t) => {
      const suggested = t.predictions
        .filter((p) => p.suggested)
        .map((p) => p.event.id);
      init[t.id] = new Set(suggested);
    });
    return init;
  });

  const handleCloseRound = async (force: boolean) => {
    const notSubmitted = teams.filter((t) => !t.submitted && !t.bankrupt);
    if (!force && notSubmitted.length > 0) {
      const names = notSubmitted.map((t) => t.name).join(', ');
      if (!confirm(`Команды не сдали решение: ${names}. Завершить принудительно?`)) {
        return;
      }
      force = true;
    }

    if (!force && !confirm('Завершить раунд? Это действие нельзя отменить.')) return;

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
    const current = new Set(eventSelections[teamId] || []);
    if (current.has(eventId)) current.delete(eventId);
    else current.add(eventId);
    setEventSelections({ ...eventSelections, [teamId]: current });
  };

  const handleApplyEvents = async (finish: boolean) => {
    const action = finish ? 'Завершить игру' : 'Применить события и перейти к следующему раунду';
    if (!confirm(`${action}? Это действие нельзя отменить.`)) return;

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
    if (!confirm('Завершить игру? Решения текущего раунда будут отброшены.')) return;
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

  const selectedTeam = selectedTeamId !== null ? teams.find((t) => t.id === selectedTeamId) : null;

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="bg-white shadow-sm border-b">
        <div className="max-w-6xl mx-auto px-4 py-4 flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold">Раунд {game.round}</h1>
            <span className="text-sm text-gray-500">
              {isOpen ? 'Приём решений' : 'Выбор событий'}
            </span>
          </div>
          <div className="flex gap-2">
            {isOpen && (
              <>
                <button
                  onClick={() => handleCloseRound(false)}
                  disabled={loading}
                  className="px-4 py-2 bg-yellow-500 text-white rounded-lg hover:bg-yellow-600 disabled:opacity-50"
                >
                  Завершить раунд
                </button>
                <button
                  onClick={handleFinishGame}
                  disabled={loading}
                  className="px-4 py-2 bg-red-500 text-white rounded-lg hover:bg-red-600 disabled:opacity-50"
                >
                  Закончить игру
                </button>
              </>
            )}
            {isClosed && (
              <>
                <button
                  onClick={() => handleApplyEvents(false)}
                  disabled={loading}
                  className="px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:opacity-50"
                >
                  Следующий раунд →
                </button>
                <button
                  onClick={() => handleApplyEvents(true)}
                  disabled={loading}
                  className="px-4 py-2 bg-red-500 text-white rounded-lg hover:bg-red-600 disabled:opacity-50"
                >
                  Последний раунд (завершить)
                </button>
              </>
            )}
          </div>
        </div>
      </header>

      <main className="max-w-6xl mx-auto px-4 py-6">
        {error && (
          <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">
            {error}
          </div>
        )}

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Список команд */}
          <div className="lg:col-span-1 space-y-3">
            <h2 className="text-lg font-semibold">Команды</h2>
            {teams.map((team) => (
              <button
                key={team.id}
                onClick={() => setSelectedTeamId(team.id)}
                className={`w-full text-left p-4 rounded-lg border-2 transition-colors ${
                  selectedTeamId === team.id
                    ? 'border-blue-500 bg-blue-50'
                    : 'border-gray-200 bg-white hover:border-gray-300'
                }`}
              >
                <div className="flex items-center justify-between">
                  <span className="font-medium">{team.name}</span>
                  <div className="flex items-center gap-2">
                    {team.bankrupt && (
                      <span className="text-xs bg-red-100 text-red-600 px-2 py-0.5 rounded">
                        банкрот
                      </span>
                    )}
                    {team.submitted ? (
                      <span className="text-green-500">✓</span>
                    ) : isOpen ? (
                      <span className="text-yellow-500">⏳</span>
                    ) : null}
                    <span
                      className={`w-2 h-2 rounded-full ${
                        team.online ? 'bg-green-500' : 'bg-gray-300'
                      }`}
                    />
                  </div>
                </div>
                {team.state && (
                  <div className="text-xs text-gray-500 mt-1">
                    💰 {team.state.budget.money} | 🔬 {team.state.budget.qp}
                  </div>
                )}
              </button>
            ))}
          </div>

          {/* Детали выбранной команды */}
          <div className="lg:col-span-2">
            {selectedTeam ? (
              <TeamDetail team={selectedTeam} isClosed={isClosed} eventSelections={eventSelections} onToggleEvent={toggleEvent} />
            ) : (
              <div className="flex items-center justify-center h-64 bg-white rounded-xl border text-gray-400">
                Выберите команду для просмотра
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}

function TeamDetail({
  team,
  isClosed,
  eventSelections,
  onToggleEvent,
}: {
  team: AdminTeam;
  isClosed: boolean;
  eventSelections: Record<number, Set<number>>;
  onToggleEvent: (teamId: number, eventId: number) => void;
}) {
  return (
    <div className="bg-white rounded-xl border p-6 space-y-6">
      <div>
        <h2 className="text-xl font-bold">{team.name}</h2>
        <div className="text-sm text-gray-500">{team.members.join(', ')}</div>
      </div>

      {/* Бюджет */}
      {team.state && (
        <div className="grid grid-cols-3 gap-3">
          <div className="p-3 bg-gray-50 rounded-lg text-center">
            <div className="text-sm text-gray-500">Деньги</div>
            <div className="text-lg font-bold">💰 {team.state.budget.money}</div>
          </div>
          <div className="p-3 bg-gray-50 rounded-lg text-center">
            <div className="text-sm text-gray-500">QP</div>
            <div className="text-lg font-bold">🔬 {team.state.budget.qp}</div>
          </div>
          <div className="p-3 bg-gray-50 rounded-lg text-center">
            <div className="text-sm text-gray-500">Макс. деньги</div>
            <div className="text-lg font-bold">{team.state.budget.maxMoney}</div>
          </div>
        </div>
      )}

      {/* Решение */}
      {team.decision && (
        <div>
          <h3 className="font-semibold mb-2">Решение</h3>
          {team.cost && (
            <div className="text-sm text-gray-600 mb-2">
              Стоимость: 💰 {team.cost.money} | 🔬 {team.cost.qp}
            </div>
          )}

          {/* Атрибуты */}
          {team.decision.attributes.length > 0 && team.state && (
            <div className="mb-3">
              <div className="text-sm font-medium text-gray-600 mb-1">Атрибуты</div>
              <div className="grid grid-cols-2 gap-2">
                {team.decision.attributes.map((a) => {
                  const meta = team.state!.configurable.find((c) => c.id === a.id);
                  return (
                    <div key={a.id} className="text-sm p-2 bg-gray-50 rounded flex justify-between">
                      <span>{meta?.name || `#${a.id}`}</span>
                      <span className="font-mono">{a.value.toFixed(2)}</span>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {/* Стратегии */}
          {team.decision.strategies.length > 0 && team.state && (
            <div className="mb-3">
              <div className="text-sm font-medium text-gray-600 mb-1">Стратегии</div>
              <div className="flex flex-wrap gap-2">
                {team.decision.strategies.map((sid) => {
                  const s = team.state!.strategies.find((s) => s.id === sid);
                  return (
                    <span key={sid} className="text-xs bg-blue-100 text-blue-700 px-2 py-1 rounded">
                      {s?.name || `#${sid}`}
                    </span>
                  );
                })}
              </div>
            </div>
          )}

          {/* Отчёт */}
          <div className="space-y-2 mt-3">
            <ReportBlock label="Почему такое решение" text={team.decision.report.why} />
            <ReportBlock label="Альтернативы" text={team.decision.report.alternative} />
            <ReportBlock label="Риски" text={team.decision.report.risks} />
            <ReportBlock label="Коррекция для реальной жизни" text={team.decision.report.correction} />
          </div>
        </div>
      )}

      {/* Прогнозы событий (только round_closed) */}
      {isClosed && team.predictions.length > 0 && (
        <div>
          <h3 className="font-semibold mb-2">События (выберите)</h3>
          <div className="space-y-2">
            {team.predictions.map((pred) => (
              <PredictionRow
                key={pred.event.id}
                prediction={pred}
                selected={(eventSelections[team.id] || new Set()).has(pred.event.id)}
                onToggle={() => onToggleEvent(team.id, pred.event.id)}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function ReportBlock({ label, text }: { label: string; text: string }) {
  if (!text) return null;
  return (
    <div className="p-3 bg-gray-50 rounded-lg">
      <div className="text-xs font-medium text-gray-500 mb-1">{label}</div>
      <div className="text-sm whitespace-pre-wrap">{text}</div>
    </div>
  );
}

function PredictionRow({
  prediction,
  selected,
  onToggle,
}: {
  prediction: Prediction;
  selected: boolean;
  onToggle: () => void;
}) {
  const { event, probability } = prediction;
  const pct = (probability * 100).toFixed(0);

  return (
    <button
      onClick={onToggle}
      className={`w-full text-left p-3 rounded-lg border-2 transition-colors ${
        selected
          ? 'border-blue-500 bg-blue-50'
          : 'border-gray-200 bg-white hover:border-gray-300'
      }`}
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span>{event.isPositive ? '✅' : '⚠️'}</span>
          <span className="font-medium">{event.name}</span>
        </div>
        <div className="flex items-center gap-3">
          <div className="text-sm text-gray-500">{pct}%</div>
          <div className={`w-5 h-5 rounded border-2 flex items-center justify-center ${
            selected ? 'border-blue-500 bg-blue-500 text-white' : 'border-gray-300'
          }`}>
            {selected && '✓'}
          </div>
        </div>
      </div>
      <p className="text-sm text-gray-600 mt-1">{event.description}</p>
    </button>
  );
}

