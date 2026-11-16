import { TeamState } from '../../types/api';

interface Props {
  state: TeamState;
}

export function TeamFinished({ state }: Props) {
  const { team, state: gameState } = state;

  if (!team) return null;

  return (
    <div className="min-h-screen bg-slate-50 flex items-center justify-center p-4">
      <div className="w-full max-w-md bg-white rounded-3xl shadow-sm border border-slate-200 p-8 text-center space-y-6">
        <div className="w-16 h-16 mx-auto rounded-full bg-slate-900 text-white flex items-center justify-center text-2xl font-bold shadow-sm">
          🏁
        </div>

        <div>
          <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">Симуляция завершена</span>
          <h1 className="text-2xl font-black text-slate-900 mt-1">{team.name}</h1>
          <p className="text-xs text-slate-500 mt-0.5">{team.members.join(', ')}</p>
        </div>

        {gameState && (
          <div className="space-y-2.5 text-left">
            <div className="flex justify-between items-center p-3.5 bg-slate-50 border border-slate-200 rounded-2xl">
              <span className="text-xs font-semibold text-slate-600">Остаток бюджета</span>
              <span className="font-bold font-mono text-emerald-600 text-sm">
                💰 {gameState.budget.money} K $
              </span>
            </div>
            <div className="flex justify-between items-center p-3.5 bg-slate-50 border border-slate-200 rounded-2xl">
              <span className="text-xs font-semibold text-slate-600">Очки исследований</span>
              <span className="font-bold font-mono text-cyan-600 text-sm">
                🔬 {gameState.budget.qp} QP
              </span>
            </div>
            {gameState.attributes.map((attr) => (
              <div key={attr.id} className="flex justify-between items-center p-3.5 bg-slate-50 border border-slate-200 rounded-2xl">
                <span className="text-xs font-semibold text-slate-600">{attr.name}</span>
                <span className="font-bold font-mono text-slate-900 text-sm">
                  {Math.round(attr.value)}
                </span>
              </div>
            ))}
          </div>
        )}

        <div className="pt-2 border-t border-slate-100 flex items-center justify-center gap-2 text-xs font-medium text-slate-400">
          <span className="w-2 h-2 rounded-full bg-indigo-500 animate-pulse" />
          <span>Спасибо за игру! Преподаватель подводит итоги...</span>
        </div>
      </div>
    </div>
  );
}
