import { useState, useEffect } from 'react';
import { api } from '../../api';
import { AdminState, Grade, SkillCode } from '../../types/api';

interface Props {
  state: AdminState;
  onUpdate: () => void;
}

const SKILLS: { code: SkillCode; label: string }[] = [
  { code: 'problem_statement', label: 'Постановка задачи' },
  { code: 'fact_checking', label: 'Проверка информации' },
  { code: 'connections', label: 'Поиск связей' },
  { code: 'alternatives', label: 'Рассмотрение альтернатив и возражений' },
  { code: 'risks', label: 'Оценка рисков' },
  { code: 'revision', label: 'Пересмотр и объяснение' },
];

export function AdminFinished({ state, onUpdate }: Props) {
  const { game, teams } = state;
  const [selectedTeamId, setSelectedTeamId] = useState<number | null>(
    teams.length > 0 ? teams[0].id : null,
  );
  const [grades, setGrades] = useState<Record<number, Grade[]>>({});
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  // Загрузить существующие оценки
  useEffect(() => {
    api.getGameDetail(game.gameId).then((detail) => {
      const initial: Record<number, Grade[]> = {};
      detail.teams.forEach((t) => {
        initial[t.id] = t.grades || [];
      });
      setGrades(initial);
    }).catch(() => {});
  }, [game.gameId]);

  const selectedTeam = selectedTeamId !== null ? teams.find((t) => t.id === selectedTeamId) : null;
  const teamGrades = selectedTeamId !== null ? grades[selectedTeamId] || [] : [];

  const getGradeValue = (skill: SkillCode): number => {
    const g = teamGrades.find((g) => g.skill === skill);
    return g ? g.score : 0;
  };

  const getGradeComment = (skill: SkillCode): string => {
    const g = teamGrades.find((g) => g.skill === skill);
    return g ? g.comment : '';
  };

  const updateGrade = (skill: SkillCode, score: number, comment: string) => {
    if (selectedTeamId === null) return;
    const existing = [...(grades[selectedTeamId] || [])];
    const idx = existing.findIndex((g) => g.skill === skill);
    const grade: Grade = { skill, score, comment };
    if (idx >= 0) existing[idx] = grade;
    else existing.push(grade);
    setGrades({ ...grades, [selectedTeamId]: existing });
  };

  const handleSave = async () => {
    if (selectedTeamId === null) return;
    setSaving(true);
    setError(null);
    setSuccess(null);
    try {
      await api.setGrades(game.gameId, selectedTeamId, {
        grades: grades[selectedTeamId] || [],
      });
      setSuccess('Оценки сохранены');
    } catch (e: unknown) {
      const err = e as Error & { userMessage?: string };
      setError(err.userMessage || err.message);
    }
    setSaving(false);
  };

  const handleNewGame = async () => {
    if (!confirm('Создать новую игру?')) return;
    try {
      await api.createGame();
      onUpdate();
    } catch (e: unknown) {
      const err = e as Error & { userMessage?: string };
      setError(err.userMessage || err.message);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="bg-white shadow-sm border-b">
        <div className="max-w-6xl mx-auto px-4 py-4 flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold">Игра завершена</h1>
            <span className="text-sm text-gray-500">
              Сценарий: {game.scenarioId} | Раундов: {game.round}
            </span>
          </div>
          <button
            onClick={handleNewGame}
            className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
          >
            Новая игра
          </button>
        </div>
      </header>

      <main className="max-w-6xl mx-auto px-4 py-6">
        {error && (
          <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">
            {error}
          </div>
        )}
        {success && (
          <div className="mb-4 p-3 bg-green-50 border border-green-200 rounded-lg text-green-700 text-sm">
            {success}
          </div>
        )}

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Выбор команды */}
          <div className="lg:col-span-1 space-y-3">
            <h2 className="text-lg font-semibold">Команды</h2>
            {teams.map((team) => (
              <button
                key={team.id}
                onClick={() => { setSelectedTeamId(team.id); setSuccess(null); }}
                className={`w-full text-left p-4 rounded-lg border-2 transition-colors ${
                  selectedTeamId === team.id
                    ? 'border-blue-500 bg-blue-50'
                    : 'border-gray-200 bg-white hover:border-gray-300'
                }`}
              >
                <div className="font-medium">{team.name}</div>
                <div className="text-sm text-gray-500">{team.members.join(', ')}</div>
              </button>
            ))}
          </div>

          {/* Оценки */}
          <div className="lg:col-span-2">
            {selectedTeam ? (
              <div className="bg-white rounded-xl border p-6">
                <h2 className="text-xl font-bold mb-6">
                  Оценки: {selectedTeam.name}
                </h2>

                <div className="space-y-6">
                  {SKILLS.map(({ code, label }) => (
                    <div key={code} className="border-b pb-4">
                      <div className="flex items-center justify-between mb-2">
                        <label className="font-medium">{label}</label>
                        <div className="flex items-center gap-2">
                          <input
                            type="range"
                            min={0}
                            max={10}
                            value={getGradeValue(code)}
                            onChange={(e) =>
                              updateGrade(code, parseInt(e.target.value), getGradeComment(code))
                            }
                            className="w-32"
                          />
                          <span className="w-8 text-center font-bold">
                            {getGradeValue(code)}
                          </span>
                        </div>
                      </div>
                      <textarea
                        value={getGradeComment(code)}
                        onChange={(e) =>
                          updateGrade(code, getGradeValue(code), e.target.value)
                        }
                        maxLength={2000}
                        placeholder="Комментарий (необязательно)"
                        rows={2}
                        className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-blue-500 outline-none resize-y"
                      />
                    </div>
                  ))}
                </div>

                <button
                  onClick={handleSave}
                  disabled={saving}
                  className="mt-6 w-full py-3 bg-green-600 text-white font-medium rounded-lg hover:bg-green-700 disabled:opacity-50"
                >
                  {saving ? 'Сохранение...' : 'Сохранить оценки'}
                </button>
              </div>
            ) : (
              <div className="flex items-center justify-center h-64 bg-white rounded-xl border text-gray-400">
                Выберите команду
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}

