import { useEffect, useState } from 'react';
import { getCategories, createCategory, deleteCategory, updateCategory, type Category } from '../api/categories';
import { Trash2, Edit2, Check, X } from 'lucide-react';

export function CategoriesWidget() {
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);
  const [newName, setNewName] = useState('');
  const [editId, setEditId] = useState<number | null>(null);
  const [editName, setEditName] = useState('');

  const fetchCategories = async () => {
    setLoading(true);
    try {
      const data = await getCategories();
      setCategories(data);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchCategories();
  }, []);

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newName.trim()) return;
    try {
      await createCategory(newName);
      setNewName('');
      fetchCategories();
    } catch (e: any) {
      alert(e.response?.data?.error || "Ошибка создания");
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm("Удалить категорию?")) return;
    try {
      await deleteCategory(id);
      fetchCategories();
    } catch (e: any) {
      alert(e.response?.data?.error || "Ошибка удаления");
    }
  };

  const handleUpdate = async (id: number) => {
    if (!editName.trim()) {
      setEditId(null);
      return;
    }
    try {
      await updateCategory(id, editName);
      setEditId(null);
      fetchCategories();
    } catch (e: any) {
      alert(e.response?.data?.error || "Ошибка обновления");
    }
  };

  if (loading) return <div className="text-gray-500">Загрузка категорий...</div>;

  return (
    <div className="w-full bg-white shadow-sm border border-gray-200 p-6 rounded-2xl space-y-4">
      <h2 className="text-xl font-bold">Категории</h2>
      
      <form onSubmit={handleAdd} className="flex gap-2">
        <input className="flex-1 border border-gray-300 rounded-lg px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500" placeholder="Новая категория" value={newName} onChange={(e) => setNewName(e.target.value)} />
        <button type="submit" className="bg-indigo-600 text-white px-4 py-2 rounded-lg hover:bg-indigo-700 font-medium whitespace-nowrap">+ Добавить</button>
      </form>

      <div className="flex flex-wrap gap-2 pt-2">
        {categories.map((cat) => (
          <div key={cat.id} className="flex items-center gap-1 bg-indigo-50 text-indigo-800 px-3 py-1.5 rounded-full text-sm font-medium border border-indigo-100">
            {editId === cat.id ? (
              <div className="flex items-center gap-1">
                <input autoFocus className="bg-white border rounded px-1 outline-none w-24 text-black" value={editName} onChange={(e) => setEditName(e.target.value)} />
                <button onClick={() => handleUpdate(cat.id)} className="text-green-600"><Check size={14} /></button>
                <button onClick={() => setEditId(null)} className="text-gray-400"><X size={14} /></button>
              </div>
            ) : (
              <>
                <span>{cat.name}</span>
                <button onClick={() => { setEditId(cat.id); setEditName(cat.name); }} className="text-indigo-400 hover:text-indigo-600 ml-1"><Edit2 size={12} /></button>
                <button onClick={() => handleDelete(cat.id)} className="text-red-400 hover:text-red-600 ml-1"><Trash2 size={12} /></button>
              </>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
