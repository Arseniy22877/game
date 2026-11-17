import { useState } from 'react';

interface Props {
  message: string;
  onEnterToken?: (token: string) => void;
}

export function TeamWaiting({ message, onEnterToken }: Props) {
  const [tokenInput, setTokenInput] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = tokenInput.trim();
    if (trimmed && onEnterToken) {
      onEnterToken(trimmed);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-50 p-4">
      <div className="max-w-md w-full bg-white rounded-3xl shadow-sm border border-slate-200 p-8 text-center space-y-5">
        <div className="w-14 h-14 mx-auto rounded-2xl bg-slate-100 border border-slate-200 flex items-center justify-center text-slate-700">
          <svg className="w-6 h-6 animate-spin text-slate-700" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
          </svg>
        </div>
        <div>
          <h2 className="text-lg font-bold text-slate-900">Режим ожидания</h2>
          <p className="text-sm text-slate-500 mt-1 leading-relaxed">{message}</p>
        </div>

        {onEnterToken && (
          <form onSubmit={handleSubmit} className="pt-2 border-t border-slate-100 space-y-2 text-left">
            <label className="block text-xs font-semibold uppercase tracking-wider text-slate-400">
              Вход по токену команды
            </label>
            <div className="flex gap-2">
              <input
                type="text"
                value={tokenInput}
                onChange={(e) => setTokenInput(e.target.value)}
                placeholder="Например: token-alpha-123"
                className="flex-1 px-3 py-2 text-xs bg-slate-50 border border-slate-200 rounded-xl focus:bg-white focus:ring-2 focus:ring-slate-900 outline-none font-mono"
              />
              <button
                type="submit"
                disabled={!tokenInput.trim()}
                className="px-4 py-2 bg-slate-900 hover:bg-slate-800 disabled:bg-slate-200 text-white text-xs font-bold rounded-xl transition-colors cursor-pointer"
              >
                Войти
              </button>
            </div>
          </form>
        )}

        <div className="pt-1 flex items-center justify-center gap-2 text-xs font-medium text-slate-400">
          <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
          <span>Подключение активно, страница обновится автоматически</span>
        </div>
      </div>
    </div>
  );
}
