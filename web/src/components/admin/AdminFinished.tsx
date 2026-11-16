import { useState, useEffect } from 'react';
import { api } from '../../api';
import { AdminState, Grade, SkillCode } from '../../types/api';

interface Props {
  state: AdminState;
  onUpdate: () => void;
}

const SKILLS: { code: SkillCode; label: string; desc: string }[] = [
  { code: 'problem_statement', label: 'Постановка задачи', desc: 'Умение определить ключевую цель и ограничения ситуации' },
  { code: 'fact_checking', label: 'Проверка информации', desc: 'Анализ доступных фактов, покупка и верификация гипотез' },
  { code: 'connections', label: 'Поиск связей', desc: 'Понимание взаимосвязи между бюджетом, стратегиями и исходами' },
  { code: 'alternatives', label: 'Рассмотрение альтернатив', desc: 'Готовность тестировать различные тактики и контр-варианты' },
  { code: 'risks', label: 'Оценка рисков', desc: 'Прогнозирование последствий и учет вероятностных факторов' },
  { code: 'revision', label: 'Пересмотр и объяснение', desc: 'Качество рефлексии и аргументации принятых решений' },
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
    const g = teamGrades.find((grade) => grade.skill === skill);
    return g ? g.score : 0;
  };

  const getGradeComment = (skill: SkillCode): string => {
    const g = teamGrades.find((grade) => grade.skill === skill);
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
      setSuccess('Оценки успешно сохранены');
    } catch (e: unknown) {
      const err = e as Error & { userMessage?: string };
      setError(err.userMessage || err.message);
    }
    setSaving(false);
  };

  const handleNewGame = async () => {
    if (!confirm('Начать новую игру? Текущая игра будет сохранена в архиве.')) return;
    try {
      await api.createGame({ confirm: true });
      onUpdate();
    } catch (e: unknown) {
      const err = e as Error & { userMessage?: string };
      setError(err.userMessage || err.message);
    }
  };

  return (
    <div className="min-h-screen bg-slate-50 flex flex-col">
      <header className="bg-white border-b border-slate-200 sticky top-0 z-20">
        <div className="max-w-7xl mx-auto px-4 py-4 flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-bold text-slate-900">Итоги симуляции и Оценки</h1>
              <span className="px-3 py-1 bg-emerald-50 text-emerald-700 border border-emerald-200 text-xs font-semibold rounded-full">
                Игра завершена
              </span>
            </div>
            <p className="text-xs text-slate-500 mt-0.5">
              Сценарий: {game.scenarioId} | Сыграно раундов: {game.round}
            </p>
          </div>
          <button
            type="button"
            onClick={handleNewGame}
            className="px-5 py-2.5 bg-slate-900 hover:bg-slate-800 text-white text-sm font-bold rounded-2xl shadow-sm transition-all"
          >
            Создать новую игру
          </button>
        </div>
      </header>

      <main className="max-w-7xl w-full mx-auto px-4 py-8 flex-1">
        {error && (
          <div className="mb-6 p-4 bg-rose-50 border border-rose-200 rounded-2xl text-rose-700 text-sm font-medium">
            {error}
          </div>
        )}
        {success && (
          <div className="mb-6 p-4 bg-emerald-50 border border-emerald-200 rounded-2xl text-emerald-700 text-sm font-medium">
            ✓ {success}
          </div>
        )}

        <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 items-start">
          {/* Список команд */}
          <div className="lg:col-span-4 space-y-3">
            <h2 className="text-base font-bold text-slate-900">Команды</h2>
            {teams.map((team) => {
              const isSelected = selectedTeamId === team.id;
              const hasGrades = (grades[team.id] || []).length > 0;

              return (
                <button
                  key={team.id}
                  type="button"
                  onClick={() => { setSelectedTeamId(team.id); setSuccess(null); }}
                  className={`w-full text-left p-5 rounded-3xl border transition-all flex items-center justify-between ${
                    isSelected
                      ? 'bg-white border-slate-900 ring-2 ring-slate-900 shadow-md'
                      : 'bg-white border-slate-200 hover:border-slate-300 shadow-sm'
                  }`}
                >
                  <div>
                    <div className="font-bold text-slate-900">{team.name}</div>
                    <div className="text-xs text-slate-500 mt-0.5">{team.members.join(', ')}</div>
                  </div>
                  {hasGrades && (
                    <span className="text-xs font-semibold text-emerald-600 bg-emerald-50 px-2.5 py-1 rounded-xl border border-emerald-200">
                      Оценено
                    </span>
                  )}
                </button>
              );
            })}
          </div>

          {/* Форма оценивания */}
          <div className="lg:col-span-8">
            {selectedTeam ? (
              <div className="bg-white border border-slate-200 rounded-3xl p-8 shadow-sm space-y-8">
                <div className="border-b border-slate-100 pb-5 flex items-center justify-between">
                  <div>
                    <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">Оценивание</span>
                    <h2 className="text-2xl font-bold text-slate-900 mt-1">{selectedTeam.name}</h2>
                  </div>
                  <button
                    type="button"
                    onClick={handleSave}
                    disabled={saving}
                    className="px-6 py-2.5 bg-slate-900 hover:bg-slate-800 text-white font-bold text-sm rounded-2xl shadow-sm transition-all disabled:opacity-50"
                  >
                    {saving ? 'Сохранение...' : 'Сохранить оценки'}
                  </button>
                </div>

                <div className="space-y-6">
                  {SKILLS.map(({ code, label, desc }) => {
                    const score = getGradeValue(code);
                    const comment = getGradeComment(code);

                    return (
                      <div key={code} className="p-5 bg-slate-50 border border-slate-200 rounded-2xl space-y-3">
                        <div className="flex items-start justify-between gap-4">
                          <div>
                            <label className="font-bold text-slate-900 text-sm block">{label}</label>
                            <p className="text-xs text-slate-400 mt-0.5">{desc}</p>
                          </div>
                          <div className="flex items-center gap-3 shrink-0">
                            <span className="text-lg font-black font-mono text-slate-900 bg-white border border-slate-200 px-3 py-1 rounded-xl shadow-2xs">
                              {score} / 10
                            </span>
                          </div>
                        </div>

                        <div className="flex items-center gap-3">
                          <input
                            type="range"
                            min={0}
                            max={10}
                            value={score}
                            onChange={(e) => updateGrade(code, parseInt(e.target.value), comment)}
                            className="flex-1 accent-slate-900 cursor-pointer"
                          />
                        </div>

                        <input
                          type="text"
                          value={comment}
                          onChange={(e) => updateGrade(code, score, e.target.value)}
                          maxLength={2000}
                          placeholder="Комментарий к оценке (необязательно)..."
                          className="w-full px-4 py-2 bg-white border border-slate-200 rounded-xl text-xs text-slate-800 placeholder-slate-400 focus:ring-2 focus:ring-slate-900 outline-none transition-all"
                        />
                      </div>
                    );
                  })}
                </div>
              </div>
            ) : (
              <div className="bg-white border border-slate-200 rounded-3xl p-12 text-center text-slate-400">
                Выберите команду для выставления оценок
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}
