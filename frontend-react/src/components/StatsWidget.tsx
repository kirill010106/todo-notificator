import { useEffect, useState } from 'react';
import { getStats, updateStats, type Stats } from '../api/stats';

export function StatsWidget() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [loading, setLoading] = useState(true);

  const fetchStats = async () => {
    setLoading(true);
    try {
      const data = await getStats();
      setStats(data);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStats();
  }, []);

  const handlePatch = async (updates: Partial<Stats>) => {
    try {
      await updateStats(updates);
      fetchStats();
    } catch (e) {
      alert('Ошибка обновления');
    }
  };

  if (loading) return <div className="text-gray-500">Загрузка статистики...</div>;
  if (!stats) return null;

  return (
    <div className="w-full bg-white shadow-sm border border-gray-200 p-6 rounded-2xl space-y-4">
      <div className="flex justify-between items-center">
        <h2 className="text-xl font-bold">Статистика профиля</h2>
        <button onClick={fetchStats} className="text-sm text-blue-500 hover:underline">Обновить</button>
      </div>
      <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
        <div className="bg-indigo-50 p-4 rounded-xl text-center">
          <div className="text-2xl font-bold text-indigo-700">{stats.points}</div>
          <div className="text-sm text-gray-600">Очки</div>
        </div>
        <div className="bg-blue-50 p-4 rounded-xl text-center">
          <div className="text-2xl font-bold text-blue-700">{stats.level}</div>
          <div className="text-sm text-gray-600">Уровень</div>
        </div>
        <div className="bg-green-50 p-4 rounded-xl text-center">
          <div className="text-2xl font-bold text-green-700">{stats.total_pomodoros}</div>
          <div className="text-sm text-gray-600">Помодоро</div>
        </div>
        <div className="bg-red-50 p-4 rounded-xl text-center">
          <div className="text-2xl font-bold text-red-700">{stats.total_burnt_tasks}</div>
          <div className="text-sm text-gray-600">Сгорело</div>
        </div>
        <div className="bg-yellow-50 p-4 rounded-xl text-center">
          <div className="text-2xl font-bold text-yellow-700">{stats.current_streak}</div>
          <div className="text-sm text-gray-600">Текущий стрик</div>
        </div>
        <div className="bg-orange-50 p-4 rounded-xl text-center">
          <div className="text-2xl font-bold text-orange-700">{stats.best_streak}</div>
          <div className="text-sm text-gray-600">Лучший стрик</div>
        </div>
      </div>
      
      <div className="border-t pt-4 mt-4 flex gap-2 flex-wrap">
        <span className="text-xs font-semibold text-gray-400 py-2 inline-block">DEBUG:</span>
        <button onClick={() => handlePatch({ total_pomodoros: stats.total_pomodoros + 1 })} className="bg-indigo-100 text-indigo-700 px-3 py-1 rounded-lg text-sm">+1 Помодоро</button>
        <button onClick={() => handlePatch({ total_burnt_tasks: stats.total_burnt_tasks + 1 })} className="bg-red-100 text-red-700 px-3 py-1 rounded-lg text-sm">+1 Сгорела</button>
        <button onClick={() => handlePatch({ points: stats.points + 10 })} className="bg-yellow-100 text-yellow-700 px-3 py-1 rounded-lg text-sm">+10 Очков</button>
      </div>
    </div>
  );
}
