interface Props {
  message: string;
}

export function TeamWaiting({ message }: Props) {
  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-50 p-4">
      <div className="max-w-md w-full bg-white rounded-3xl shadow-sm border border-slate-200 p-8 text-center space-y-4">
        <div className="w-14 h-14 mx-auto rounded-2xl bg-slate-100 border border-slate-200 flex items-center justify-center text-slate-800 text-2xl font-bold">
          ⏳
        </div>
        <h2 className="text-lg font-bold text-slate-900">Режим ожидания</h2>
        <p className="text-sm text-slate-500 leading-relaxed">{message}</p>
        <div className="pt-2 flex items-center justify-center gap-2 text-xs font-medium text-slate-400">
          <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
          <span>Подключение активно, страница обновится автоматически</span>
        </div>
      </div>
    </div>
  );
}
