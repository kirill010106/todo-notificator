import { useState } from 'react';
import { type Task, updateTask, deleteTask } from '../api/tasks';
import { startPomodoro } from '../api/pomodoro';
import { usePomodoro } from '../store/usePomodoro';
import { useToast } from '../store/useToast';
import { Play, CheckCircle, Circle, Edit2, Trash2, Save, X, Flame } from 'lucide-react';

export function TaskItem({ task, onUpdate }: { task: Task, onUpdate: () => void }) {
  const [isEditing, setIsEditing] = useState(false);
  const [editTitle, setEditTitle] = useState(task.title);
  const [editDesc, setEditDesc] = useState(task.description || '');

  const { setActiveSession } = usePomodoro();
  const { addToast } = useToast();

  const handleSave = async () => {
    try {
      await updateTask(task.id, { title: editTitle, description: editDesc });
      setIsEditing(false);
      onUpdate();
      addToast('Задача обновлена', 'success');
    } catch (e: any) {
      addToast(e.response?.data?.error || 'Ошибка обновления', 'error');
    }
  };

  const handleComplete = async () => {
    const newStatus = task.status === 'done' ? 'pending' : 'done';
    try {
      await updateTask(task.id, { status: newStatus });
      onUpdate();
      addToast(newStatus === 'pending' ? 'Задача возвращена в работу' : 'Задача выполнена!', 'success');
    } catch (e: any) {
      addToast(e.response?.data?.error || 'Ошибка при изменении статуса', 'error');
    }
  };

  const handleDelete = async () => {
    if (!confirm('Удалить задачу?')) return;
    try {
      await deleteTask(task.id);
      onUpdate();
      addToast('Задача удалена', 'info');
    } catch (e: any) {
      addToast(e.response?.data?.error, 'error');
    }
  };

  const handleStart = async () => {
    try {
      const data = await startPomodoro(task.id);
      setActiveSession(data.session);
      addToast('Фокус на задаче запущен!', 'success');      
      document.getElementById('pomodoro-widget')?.scrollIntoView({ behavior: 'smooth' });
    } catch (e: any) {
      if (e.response?.status === 409 && e.response.data?.active_session) {      
        if (confirm('У вас уже есть активный таймер. Продолжить его?')) {
          setActiveSession(e.response.data.active_session);
          addToast('Восстановлен активный таймер ⏱️', 'info');
          document.getElementById('pomodoro-widget')?.scrollIntoView({ behavior: 'smooth' });
        }
      } else {
        addToast(e.response?.data?.error || 'Ошибка запуска таймера', 'error');
      }
    }
  };

  if (isEditing) {
    return (
      <div className="p-4 border border-indigo-200 rounded-2xl bg-indigo-50/10 flex flex-col gap-3 shadow-sm ring-4 ring-indigo-50/50">
        <input
          className="border-2 border-indigo-100 rounded-xl px-4 py-2 outline-none focus:border-indigo-500 focus:ring-4 focus:ring-indigo-500/10 transition-all font-medium text-gray-900"
          value={editTitle}
          onChange={(e) => setEditTitle(e.target.value)}
          placeholder="Что нужно сделать?"
          autoFocus
        />
        <textarea 
          className="border-2 border-gray-100 rounded-xl px-4 py-2 outline-none focus:border-indigo-500 focus:ring-4 focus:ring-indigo-500/10 transition-all text-sm text-gray-700 resize-none placeholder:text-gray-400"
          value={editDesc}
          onChange={(e) => setEditDesc(e.target.value)}
          placeholder="Детали (если нужны)"
          rows={2}
        />
        <div className="flex gap-2 justify-end mt-2">
          <button onClick={() => setIsEditing(false)} className="text-gray-500 hover:bg-gray-100 px-4 py-2 rounded-xl flex items-center gap-1.5 font-medium transition-colors">
            <X size={16} /> Отмена
          </button>
          <button onClick={handleSave} className="bg-indigo-600 shadow-md shadow-indigo-600/20 text-white hover:bg-indigo-700 px-5 py-2 rounded-xl flex items-center gap-1.5 font-medium transition-all hover:-translate-y-0.5">
            <Save size={16} /> Сохранить
          </button>
        </div>
      </div>
    );
  }

  const isBurnt = task.status === 'burnt';
  const isCompleted = task.status === 'done';

  return (
    <div className={`group p-5 border rounded-2xl flex flex-col gap-3 transition-all ${
      isBurnt ? 'bg-red-50/50 border-red-200 hover:bg-red-50 hover:shadow-sm hover:border-red-300' : 'bg-white hover:bg-gray-50 hover:shadow-sm hover:border-gray-300'
    } ${isCompleted ? 'opacity-60 grayscale hover:opacity-100 transition-opacity' : ''}`}>
      
      <div className="flex gap-4 items-start">
        <button
          onClick={handleComplete}
          className={`mt-0.5 transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 rounded-full ${isBurnt ? 'focus:ring-red-500' : 'focus:ring-indigo-500'}`}
        >
          {isCompleted ? <CheckCircle size={24} className="fill-green-50 text-green-500" /> : <Circle size={24} className={isBurnt ? 'text-red-400' : 'text-gray-400'} />}
        </button>

        <div className="flex-1 min-w-0">
          <div className="flex justify-between items-start gap-4">
            <h3 className={`font-semibold text-lg max-w-[80%] break-words ${isCompleted ? 'line-through text-gray-500' : 'text-gray-900'} ${isBurnt ? 'text-red-900' : ''}`}>   
              {task.title}
            </h3>

            <div className="flex items-center gap-1.5 shrink-0 opacity-80 group-hover:opacity-100 transition-opacity">
              {isBurnt && !isCompleted && (
                <span className="flex items-center gap-1 text-[10px] font-bold uppercase tracking-widest px-2.5 py-1 bg-red-100 text-red-600 rounded-lg border border-red-200 shadow-sm">
                  <Flame size={12} fill="currentColor" /> Сгорела
                </span>
              )}
              <button
                  onClick={() => setIsEditing(true)}
                  className={`p-1.5 text-gray-400 hover:text-gray-800 hover:bg-white rounded-full transition-colors shadow-sm ${isBurnt ? 'hover:text-red-600 hover:bg-red-100' : ''}`}
              >
                <Edit2 size={16} />
              </button>
              <button
                onClick={handleDelete}
                className="p-1.5 text-gray-400 hover:text-red-500 hover:bg-white rounded-full transition-colors shadow-sm"
              >
                <Trash2 size={16} />
              </button>
            </div>
          </div>

          {task.description && (
             <p className={`text-sm mt-2 mb-3 leading-relaxed break-words ${isBurnt ? 'text-red-700/70' : 'text-gray-500'}`}>
               {task.description}
             </p>
          )}

          <div className="flex justify-between items-end mt-4 pt-3 border-t border-gray-100">
            <div className={`flex items-center gap-1.5 text-xs font-semibold ${isBurnt ? 'text-red-500' : 'text-gray-400'}`}>
              {(task.pomodoros_spent ?? 0) > 0 && (
                <span title="Потрачено сессий по 25 мин" className="text-orange-500 font-bold opacity-90 text-[11px] uppercase tracking-wider bg-orange-50 px-2 py-0.5 rounded border border-orange-100">
                  🍅 {task.pomodoros_spent}
                </span>
              )}
            </div>
            {!isCompleted && !isBurnt && (
              <button
                onClick={handleStart}
                className="group/btn flex items-center gap-1.5 text-sm bg-indigo-50 text-indigo-700 hover:bg-indigo-100 hover:text-indigo-900 px-4 py-2 rounded-xl transition-colors font-semibold active:bg-indigo-200"
              >
                <Play size={14} fill="currentColor" className="text-indigo-500 group-hover/btn:text-indigo-700" /> Начать Помодоро
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}