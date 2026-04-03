import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { PomodoroSession } from '../api/pomodoro';

interface BreakState {
  type: 'short' | 'long';
  startTime: string;
}

interface PomodoroState {
  activeSession: PomodoroSession | null;
  completedPomodorosCount: number;
  interSessionBreak: BreakState | null;
  setActiveSession: (session: PomodoroSession | null) => void;
  takeBreak: (session: PomodoroSession) => void;
  clearSession: () => void;
  incrementCompleted: () => void;
  resetCompleted: () => void;
  setInterSessionBreak: (brk: BreakState | null) => void;
}

export const usePomodoro = create<PomodoroState>()(
  persist(
    (set) => ({
      activeSession: null,
      completedPomodorosCount: 0,
      interSessionBreak: null,
      setActiveSession: (session) => set({ activeSession: session, interSessionBreak: null }),
      takeBreak: (session) => set({ activeSession: session }),
      clearSession: () => set({ activeSession: null }),
      incrementCompleted: () => set((state) => ({ completedPomodorosCount: state.completedPomodorosCount + 1 })),
      resetCompleted: () => set({ completedPomodorosCount: 0 }),
      setInterSessionBreak: (brk) => set({ interSessionBreak: brk })
    }),
    {
      name: 'pomodoro-storage',
      partialize: (state) => ({ 
        completedPomodorosCount: state.completedPomodorosCount,
        interSessionBreak: state.interSessionBreak
      }),
    }
  )
);