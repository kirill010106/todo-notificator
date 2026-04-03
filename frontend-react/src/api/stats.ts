import { api } from './client';

export interface Stats {
  points: number;
  level: number;
  total_pomodoros: number;
  total_burnt_tasks: number;
  current_streak: number;
  best_streak: number;
}

export const getStats = async (): Promise<Stats> => {
  const { data } = await api.get('/me/stats');
  return data.stats;
};

export const updateStats = async (updates: Partial<Stats>): Promise<void> => {
  await api.patch('/me/stats', updates);
};
