import { useState } from 'react';
import { api } from '../../api';
import { TeamSummary } from '../../types/api';

interface Props {
  team: TeamSummary | null;
  onRegistered: () => void;
}

/**
 * Лобби команды: регистрация (если нет токена) или ожидание старта.
 */
export function TeamLobby({ team, onRegistered }: Props) {
  const hasToken = !!api.getTeamToken();

  if (hasToken && team) {
    return <LobbyReady team={team} onUpdate={onRegistered} />;
  }

  return <RegistrationForm onRegistered={onRegistered} />;
}

function RegistrationForm({ onRegistered }: { onRegistered: () => void }) {
  const [name, setName] = useState('');
  const [members, setMembers] = useState<string[]>(['']);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const addMember = () => {
    if (members.length < 10) {
      setMembers([...members, '']);
    }
  };

  const removeMember = (index: number) => {
    if (members.length > 1) {
      setMembers(members.filter((_, i) => i !== index));
    }
  };

  const updateMember = (index: number, value: string) => {
    const updated = [...members];
    updated[index] = value;
    setMembers(updated);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    const trimmedName = name.trim();
    const trimmedMembers = members.map((m) => m.trim()).filter((m) => m.length > 0);

    if (!trimmedName) {
      setError('Введите название команды');
      return;
    }
    if (trimmedMembers.length === 0) {
      setError('Добавьте хотя бы одного участника');
      return;
    }

    setSubmitting(true);
    try {
      const res = await api.registerTeam({
        name: trimmedName,
        members: trimmedMembers,
      });
      api.setTeamToken(res.token);
      onRegistered();
    } catch (e: unknown) {
      const err = e as Error & { userMessage?: string };
      setError(err.userMessage || err.message);
    }
    setSubmitting(false);
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <form onSubmit={handleSubmit} className="w-full max-w-md bg-white rounded-2xl shadow-lg p-8">
        <h1 className="text-2xl font-bold text-center mb-6">Регистрация команды</h1>

        {error && (
          <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">
            {error}
          </div>
        )}

        <div className="mb-6">
          <label className="block text-sm font-medium text-gray-700 mb-1">
            Название команды
          </label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={40}
            placeholder="Введите название..."
            className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none"
          />
        </div>

        <div className="mb-6">
          <label className="block text-sm font-medium text-gray-700 mb-2">
            Участники
          </label>
          {members.map((member, i) => (
            <div key={i} className="flex gap-2 mb-2">
              <input
                type="text"
                value={member}
                onChange={(e) => updateMember(i, e.target.value)}
                maxLength={60}
                placeholder={`Участник ${i + 1}`}
                className="flex-1 px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none"
              />
              {members.length > 1 && (
                <button
                  type="button"
                  onClick={() => removeMember(i)}
                  className="px-3 py-2 text-red-500 hover:bg-red-50 rounded-lg"
                >
                  ✕
                </button>
              )}
            </div>
          ))}
          {members.length < 10 && (
            <button
              type="button"
              onClick={addMember}
              className="text-sm text-blue-600 hover:text-blue-800"
            >
              + Добавить участника
            </button>
          )}
        </div>

        <button
          type="submit"
          disabled={submitting}
          className="w-full py-3 bg-blue-600 text-white font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {submitting ? 'Регистрация...' : 'Готовы!'}
        </button>
      </form>
    </div>
  );
}

function LobbyReady({ team, onUpdate }: { team: TeamSummary; onUpdate: () => void }) {
  const [toggling, setToggling] = useState(false);

  const toggleReady = async () => {
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
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <div className="w-full max-w-md bg-white rounded-2xl shadow-lg p-8 text-center">
        <div className={`w-20 h-20 mx-auto mb-4 rounded-full flex items-center justify-center ${
          team.ready ? 'bg-green-100' : 'bg-yellow-100'
        }`}>
          <span className="text-3xl">{team.ready ? '✓' : '⏳'}</span>
        </div>

        <h1 className="text-2xl font-bold mb-2">{team.name}</h1>

        <div className="text-gray-500 mb-6">
          {team.members.join(', ')}
        </div>

        <div className={`inline-block px-4 py-2 rounded-full text-sm font-medium mb-6 ${
          team.ready
            ? 'bg-green-100 text-green-700'
            : 'bg-yellow-100 text-yellow-700'
        }`}>
          {team.ready ? 'Готовы к игре!' : 'Не готовы'}
        </div>

        <div>
          <button
            onClick={toggleReady}
            disabled={toggling}
            className={`w-full py-3 font-medium rounded-lg transition-colors ${
              team.ready
                ? 'bg-gray-200 text-gray-700 hover:bg-gray-300'
                : 'bg-green-600 text-white hover:bg-green-700'
            } disabled:opacity-50`}
          >
            {team.ready ? 'Снять готовность' : 'Мы готовы!'}
          </button>
        </div>

        <p className="mt-6 text-sm text-gray-400">
          Ожидание начала игры от преподавателя...
        </p>
      </div>
    </div>
  );
}

