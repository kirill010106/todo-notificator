import { api } from './client';

export interface Task {
  id: number;
  title: string;
  description?: string;
  priority?: string;
  deadline?: string;
  reminder_at?: string;
  category_id?: number | null;
  status: string;
  pomodoros_spent?: number;
  is_completed: boolean;
}

export const getTasks = async (offset = 0, limit = 100): Promise<Task[]> => {
  const { data } = await api.get('/tasks', { params: { offset, limit } });
  return data.tasks || [];
};

export const createTask = async (task: Partial<Task>): Promise<number> => {
  const { data } = await api.post('/tasks', task);
  return data.task_id;
};

export const updateTask = async (id: number, updates: Partial<Task>): Promise<void> => {
  await api.patch('/tasks/' + id, updates);
};

export const deleteTask = async (id: number): Promise<void> => {
  await api.delete('/tasks/' + id);
};
