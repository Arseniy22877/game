import { TeamState } from '../../types/api';

interface Props {
  state: TeamState;
}

export function TeamFinished({ state }: Props) {
  const { team, state: gameState } = state;

  if (!team) return null;

  return (
    <div className="min-h-screen bg-gray-50 flex items-center justify-center px-4">
      <div className="w-full max-w-md bg-white rounded-2xl shadow-lg p-8 text-center">
        <div className="w-20 h-20 mx-auto mb-4 rounded-full bg-blue-100 flex items-center justify-center">
          <span className="text-3xl">🏁</span>
        </div>

        <h1 className="text-2xl font-bold mb-2">Игра завершена!</h1>
        <p className="text-gray-500 mb-6">{team.name}</p>

        {gameState && (
          <div className="text-left space-y-3 mb-6">
            <div className="flex justify-between p-3 bg-gray-50 rounded-lg">
              <span className="text-gray-600">Остаток бюджета</span>
              <span className="font-medium">💰 {gameState.budget.money}</span>
            </div>
            <div className="flex justify-between p-3 bg-gray-50 rounded-lg">
              <span className="text-gray-600">Очки QP</span>
              <span className="font-medium">🔬 {gameState.budget.qp}</span>
            </div>
            {gameState.attributes.map((attr) => (
              <div key={attr.id} className="flex justify-between p-3 bg-gray-50 rounded-lg">
                <span className="text-gray-600">{attr.name}</span>
                <span className="font-medium">{attr.value.toFixed(2)}</span>
              </div>
            ))}
          </div>
        )}

        <p className="text-sm text-gray-400">
          Спасибо за участие! Преподаватель выставляет оценки.
        </p>
      </div>
    </div>
  );
}

