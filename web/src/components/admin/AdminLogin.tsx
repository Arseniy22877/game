import { useState } from 'react';

interface Props {
  onLogin: (token: string) => void;
}

export function AdminLogin({ onLogin }: Props) {
  const [token, setToken] = useState('admin');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = token.trim();
    if (trimmed) {
      onLogin(trimmed);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-50 px-4">
      <form onSubmit={handleSubmit} className="w-full max-w-sm bg-white rounded-3xl shadow-sm border border-slate-200 p-8 space-y-6">
        <div className="text-center">
          <div className="w-12 h-12 mx-auto mb-3 rounded-2xl bg-slate-900 text-white flex items-center justify-center shadow-sm">
            <svg className="w-6 h-6 text-slate-100" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
            </svg>
          </div>
          <h1 className="text-xl font-bold text-slate-900">Вход для преподавателя</h1>
          <p className="text-xs text-slate-500 mt-1">Управление сессиями симуляции</p>
        </div>

        <div>
          <label className="block text-xs font-semibold text-slate-500 uppercase tracking-wider mb-2">
            Токен администратора
          </label>
          <input
            type="password"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder="Введите ADMIN_TOKEN..."
            className="w-full px-4 py-3 bg-slate-50 border border-slate-200 rounded-2xl text-slate-900 placeholder-slate-400 font-medium focus:bg-white focus:ring-2 focus:ring-slate-900 outline-none transition-all"
          />
          <p className="text-xs text-slate-400 mt-1.5">Для локального тестирования: admin</p>
        </div>

        <button
          type="submit"
          className="w-full py-3.5 bg-slate-900 hover:bg-slate-800 text-white font-bold rounded-2xl transition-all shadow-sm"
        >
          Войти в панель
        </button>
      </form>
    </div>
  );
}
