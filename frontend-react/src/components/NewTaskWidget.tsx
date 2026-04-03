import { useState, useEffect } from 'react';
import { createTask } from '../api/tasks';
import { getCategories, type Category } from '../api/categories';

export function NewTaskWidget({ onTaskCreated }: { onTaskCreated: () => void }) {
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [categoryId, setCategoryId] = useState<number | undefined>(undefined);
  const [deadline, setDeadline] = useState('');
  const [categories, setCategories] = useState<Category[]>([]);

  useEffect(() => {
    getCategories().then(setCategories).catch(console.error);
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;
    
    const payload: any = { title };
    if (description) payload.description = description;
    if (categoryId) payload.category_id = categoryId;
    if (deadline) payload.deadline = new Date(deadline).toISOString();

    try {
      await createTask(payload);
      setTitle('');
      setDescription('');
      setCategoryId(undefined);
      setDeadline('');
      onTaskCreated();
    } catch (err: any) {
      alert(err.response?.data?.error || "Ошибка создания задачи");
    }
  };

  return (
    <div className="bg-white shadow-sm border border-gray-200 p-6 rounded-2xl w-full">
      <h2 className="text-xl font-bold mb-4">Новая задача</h2>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Что нужно сделать?</label>
          <input 
            className="w-full border border-gray-300 rounded-lg px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500"
            placeholder="Название задачи"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            required
          />
        </div>
        
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Описание (необязательно)</label>
          <textarea 
            className="w-full border border-gray-300 rounded-lg px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500"
            placeholder="Дополнительные детали"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
          />
        </div>

        <div className="flex gap-4">
          <div className="flex-1">
            <label className="block text-sm font-medium text-gray-700 mb-1">Категория</label>
            <select 
              className="w-full border border-gray-300 rounded-lg px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 bg-white"
              value={categoryId || ''}
              onChange={(e) => setCategoryId(e.target.value ? Number(e.target.value) : undefined)}
            >
              <option value="">Без категории</option>
              {categories.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </div>
          <div className="flex-1">
            <label className="block text-sm font-medium text-gray-700 mb-1">Дедлайн</label>
            <input 
              type="datetime-local"
              className="w-full border border-gray-300 rounded-lg px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500"
              value={deadline}
              onChange={(e) => setDeadline(e.target.value)}
            />
          </div>
        </div>

        <button 
          type="submit"
          className="w-full bg-indigo-600 text-white font-medium py-2.5 rounded-lg hover:bg-indigo-700 transition-colors"
        >
          Добавить задачу
        </button>
      </form>
    </div>
  );
}
