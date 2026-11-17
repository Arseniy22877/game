import { useState } from 'react';
import { api } from '../../api';
import { TeamSummary } from '../../types/api';

interface Props {
  team: TeamSummary | null;
  onRegistered: () => void;
}

/**
 * Экран 2: Лобби команды (регистрация и переключение готовности).
 * Полностью воспроизводит макет страницы 1 PDF.
 */
export function TeamLobby({ team, onRegistered }: Props) {
  if (team) {
    return <TeamReadyView team={team} onUpdate={onRegistered} />;
  }

  return <TeamRegistrationView onRegistered={onRegistered} />;
}

function TeamRegistrationView({ onRegistered }: { onRegistered: () => void }) {
  const [name, setName] = useState('');
  const [members, setMembers] = useState<string[]>([]);
  const [currentMemberInput, setCurrentMemberInput] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleAddMember = () => {
    const trimmed = currentMemberInput.trim();
    if (!trimmed) return;
    if (members.length >= 10) {
      setError('Максимум 10 участников в команде');
      return;
    }
    setMembers([...members, trimmed]);
    setCurrentMemberInput('');
    setError(null);
  };

  const handleRemoveMember = (idx: number) => {
    setMembers(members.filter((_, i) => i !== idx));
  };

  const handleSubmit = async (isReady: boolean) => {
    setError(null);
    const trimmedName = name.trim();
    const finalMembers = [...members];
    if (currentMemberInput.trim()) {
      finalMembers.push(currentMemberInput.trim());
    }

    if (!trimmedName) {
      setError('Введите название команды');
      return;
    }
    if (finalMembers.length === 0) {
      setError('Добавьте хотя бы одного участника');
      return;
    }

    setSubmitting(true);
    try {
      const res = await api.registerTeam({
        name: trimmedName,
        members: finalMembers,
      });
      api.setTeamToken(res.token);

      if (isReady) {
        await api.setReady({ ready: true });
      }

      onRegistered();
    } catch (e: unknown) {
      const err = e as Error & { userMessage?: string };
      setError(err.userMessage || err.message);
    }
    setSubmitting(false);
  };

  return (
    <div className="min-h-screen bg-slate-50 flex items-center justify-center p-4">
      <div className="max-w-md w-full bg-white rounded-3xl shadow-sm border border-slate-200 p-8 space-y-6">
        <div className="text-center">
          <h1 className="text-2xl font-bold tracking-tight text-slate-900">Вход в симуляцию</h1>
          <p className="text-sm text-slate-500 mt-1">Зарегистрируйте команду для участия в игре</p>
        </div>

        {error && (
          <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-red-600 text-sm">
            {error}
          </div>
        )}

        {/* Название команды */}
        <div>
          <label className="block text-xs font-semibold text-slate-500 uppercase tracking-wider mb-2">
            Название команды
          </label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={40}
            placeholder="Введите название команды..."
            className="w-full px-4 py-3 bg-slate-50 border border-slate-200 rounded-xl text-slate-900 placeholder-slate-400 font-medium focus:bg-white focus:ring-2 focus:ring-slate-900 focus:border-transparent outline-none transition-all"
          />
        </div>

        {/* Список добавленных участников */}
        <div>
          <label className="block text-xs font-semibold text-slate-500 uppercase tracking-wider mb-2">
            Состав команды ({members.length})
          </label>
          <div className="space-y-2 mb-3">
            {members.map((m, idx) => (
              <div
                key={idx}
                className="flex items-center justify-between px-4 py-2.5 bg-slate-50 border border-slate-200 rounded-xl text-sm font-medium text-slate-800"
              >
                <span>{m}</span>
                <button
                  type="button"
                  onClick={() => handleRemoveMember(idx)}
                  className="text-slate-400 hover:text-red-500 transition-colors"
                >
                  ✕
                </button>
              </div>
            ))}
          </div>

          {/* Инпут добавления участника */}
          {members.length < 10 && (
            <div className="space-y-2">
              <input
                type="text"
                value={currentMemberInput}
                onChange={(e) => setCurrentMemberInput(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); handleAddMember(); } }}
                maxLength={60}
                placeholder="Введите ФИО участника..."
                className="w-full px-4 py-2.5 bg-slate-50 border border-slate-200 rounded-xl text-slate-900 placeholder-slate-400 text-sm focus:bg-white focus:ring-2 focus:ring-slate-900 focus:border-transparent outline-none transition-all"
              />
              <button
                type="button"
                onClick={handleAddMember}
                className="w-full py-2 bg-slate-100 hover:bg-slate-200 text-slate-700 text-sm font-medium rounded-xl transition-colors"
              >
                + Добавить участника
              </button>
            </div>
          )}
        </div>

        {/* Кнопка готовности / отправки */}
        <div className="pt-2">
          <button
            type="button"
            disabled={submitting}
            onClick={() => handleSubmit(true)}
            className="w-full py-3.5 bg-slate-900 hover:bg-slate-800 text-white font-semibold rounded-2xl shadow-sm transition-all disabled:opacity-50"
          >
            {submitting ? 'Регистрация...' : 'Готовы к игре'}
          </button>
        </div>
      </div>
    </div>
  );
}

function TeamReadyView({ team, onUpdate }: { team: TeamSummary; onUpdate: () => void }) {
  const [toggling, setToggling] = useState(false);

  const handleToggle = async () => {
    setToggling(true);
    try {
      await api.setReady({ ready: !team.ready });
      onUpdate();
    } catch (e: unknown) {
      console.error(e);
    }
    setToggling(false);
  };

  return (
    <div className="min-h-screen bg-slate-50 flex items-center justify-center p-4">
      <div className="max-w-md w-full bg-white rounded-3xl shadow-sm border border-slate-200 p-8 text-center space-y-6">
        {/* Индикатор текущего статуса */}
        <div className="flex flex-col items-center gap-2">
          <div className="w-16 h-16 mx-auto rounded-full flex items-center justify-center text-2xl font-bold bg-slate-100 border border-slate-200">
            {team.ready ? (
              <span className="text-emerald-600">✓</span>
            ) : (
              <span className="text-rose-500 font-bold">✕</span>
            )}
          </div>
          <div className={`text-sm font-bold ${team.ready ? 'text-emerald-600' : 'text-slate-500'}`}>
            {team.ready ? '✓ Команда готова к игре' : '✕ Команда не готова'}
          </div>
        </div>

        <div>
          <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">Команда</span>
          <h1 className="text-2xl font-bold text-slate-900 mt-0.5">{team.name}</h1>
        </div>

        {/* Список участников */}
        <div className="bg-slate-50 border border-slate-200 rounded-2xl p-4 text-left">
          <div className="text-xs font-semibold uppercase tracking-wider text-slate-400 mb-2">Участники</div>
          <div className="space-y-1.5">
            {team.members.map((m, idx) => (
              <div key={idx} className="flex items-center gap-2 text-sm font-medium text-slate-700">
                <span className="w-1.5 h-1.5 rounded-full bg-slate-400" />
                <span>{m}</span>
              </div>
            ))}
          </div>
        </div>

        {/* Кнопка-переключатель статуса:
            Если команда НЕ готова -> кнопка «Готовы» (темная), нажатие подтверждает готовность.
            Если команда ГОТОВА -> кнопка «Не готовы» (светлая), нажатие снимает готовность. */}
        <div>
          <button
            type="button"
            onClick={handleToggle}
            disabled={toggling}
            className={`w-full py-3.5 font-semibold rounded-2xl transition-all shadow-sm ${
              !team.ready
                ? 'bg-slate-900 text-white hover:bg-slate-800'
                : 'bg-white border-2 border-slate-300 text-slate-700 hover:bg-slate-50'
            }`}
          >
            {!team.ready ? 'Готовы' : 'Не готовы'}
          </button>
          <div className="text-xs text-slate-400 mt-2">
            {!team.ready ? 'Нажмите, чтобы подтвердить готовность' : 'Нажмите, чтобы снять готовность'}
          </div>
        </div>

        <div className="pt-2 border-t border-slate-100 flex flex-col items-center justify-center gap-2 text-xs font-medium text-slate-500">
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
            <span>Ожидание начала игры преподавателем...</span>
          </div>
          <button
            type="button"
            onClick={() => {
              if (confirm('Выйти из этой команды и зарегистрировать другую?')) {
                api.clearTeamToken();
                onUpdate();
              }
            }}
            className="text-slate-400 hover:text-slate-600 mt-1 underline"
          >
            Сменить команду
          </button>
        </div>
      </div>
    </div>
  );
}
