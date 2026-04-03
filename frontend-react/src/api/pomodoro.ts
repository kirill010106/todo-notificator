import { api } from './client';

export interface PomodoroSession {
  id: number;
  user_id: number;
  task_id: number | null;
  started_at: string;
  duration_minutes: number;
  status: string;
  breaks_used: number;
}

export const loginUser = async (email: string, password: string) => {
  const { data } = await api.post('/login', { email, password });
  return { accessToken: data.access_token, refreshToken: data.refresh_token };
};

export const startPomodoro = async (taskId?: number) => {
  const { data } = await api.post('/pomodoros/start', { task_id: taskId });
  return data;
};

export const pausePomodoro = async (id: number) => {
  const { data } = await api.post(`/pomodoros/${id}/pause`);
  return data;
};

export const completePomodoro = async (id: number, status: 'completed' | 'abandoned' | 'burnt') => {
  const { data } = await api.post(`/pomodoros/${id}/stop`, { action: status });
  return data;
};
