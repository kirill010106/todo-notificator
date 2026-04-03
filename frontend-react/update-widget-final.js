const fs = require('fs');
const content = "import { useState, useEffect } from 'react';
import { usePomodoro } from '../store/usePomodoro';
import { pausePomodoro, completePomodoro, startPomodoro } from '../api/pomodoro';
import { differenceInSeconds } from 'date-fns';
import { Modal } from './ui/Modal';
import { useToast } from '../store/useToast';
import { type Task, updateTask } from '../api/tasks';
import { Play, Bug, Flame, Coffee, CheckCircle, SkipForward } from 'lucide-react';

export function PomodoroWidget({ tasks, onTaskUpdated }: { tasks: Task[], onTaskUpdated: () => void }) {
  const { 
    activeSession, setActiveSession, takeBreak, clearSession, 
    completedPomodorosCount, incrementCompleted, interSessionBreak, setInterSessionBreak
  } = usePomodoro();

  const [timeLeft, setTimeLeft] = useState<number>(0);
  const [mode, setMode] = useState<'focus' | 'break'>('focus');
  const [breakStartTime, setBreakStartTime] = useState<Date | null>(null);
  
  const { addToast } = useToast();
  const [isStopModalOpen, setIsStopModalOpen] = useState(false);
  const [stopAction, setStopAction] = useState<'completed' | 'abandoned' | null>(null);
  const [markTaskComplete, setMarkTaskComplete] = useState(true);
  const [isDebugMode, setIsDebugMode] = useState(false);

  const focusedTask = activeSession?.task_id ? tasks.find(t => t.id === activeSession.task_id) : null;
  const cycleBoxes = [1, 2, 3, 4];
  const currentCycleProgress = completedPomodorosCount % 4;

  useEffect(() => {
    let interval: NodeJS.Timeout;
    if (activeSession) {
      const startTime = mode === 'focus' ? new Date(activeSession.started_at) : (breakStartTime || new Date());
      if (mode === 'break' && !breakStartTime) setBreakStartTime(startTime);

      const durationSec = isDebugMode
        ? (mode === 'focus' ? 10 : 5) 
        : (mode === 'focus' ? activeSession.duration_minutes * 60 : 5 * 60);

      interval = setInterval(() => {
        const elapsed = differenceInSeconds(new Date(), startTime);
        let remaining = Math.max(durationSec - elapsed, 0);
        if (Number.isNaN(remaining)) remaining = 0;
        setTimeLeft(remaining);

        if (remaining === 0) {
          if (mode === 'break') {
            setMode('focus');
            setBreakStartTime(null);
            addToast('Кофе-брейк окончен! Возвращаемся к работе.', 'success');
          } else {
            clearInterval(interval);
            addToast('Время фокуса вышло! Пора завершить задачу.', 'info');
          }
        }
      }, 1000);
    } else if (interSessionBreak) {
      const start = new Date(interSessionBreak.startTime);
      const isLong = interSessionBreak.type === 'long';
      const durationSec = isDebugMode ? 5 : (isLong ? 15 * 60 : 5 * 60);

      interval = setInterval(() => {
        const elapsed = differenceInSeconds(new Date(), start);
        let remaining = Math.max(durationSec - elapsed, 0);
        if (Number.isNaN(remaining)) remaining = 0;
        setTimeLeft(remaining);

        if (remaining === 0) {
          clearInterval(interval);
          setInterSessionBreak(null);
          addToast(isLong ? 'Длинный перерыв окончен, с новыми силами!' : 'Короткий перерыв окончен!', 'success');
        }
      }, 1000);
    }
    return () => clearInterval(interval);
  }, [activeSession, mode, breakStartTime, isDebugMode, interSessionBreak]);

  const handleGenericStart = async () => {
    try {
        const data = await startPomodoro();
        setActiveSession(data.session);
        setMode('focus');
        setBreakStartTime(null);
        setInterSessionBreak(null);
        addToast('Таймер запущен!', 'success');
    } catch (e: any) {
        addToast('Ошибка запуска', 'error');
    }
  };

  const handleTakeCoffeeBreak = async () => {
    try {
      await pausePomodoro(activeSession!.id);
      takeBreak({ ...activeSession!, breaks_used: 1 });
      setMode('break');
      addToast('Время отдохнуть 5 минут ☕', 'info');
    } catch (e: any) {
      addToast('Ошибка при взятии перерыва', 'error');
    }
  };

  const confirmStop = async () => {
    if (!stopAction || !activeSession) return;
    try {
      if (stopAction === 'completed') {
          await completePomodoro(activeSession.id, 'completed');
          incrementCompleted(); 
          const newTotal = completedPomodorosCount + 1;
          setInterSessionBreak({ type: newTotal % 4 === 0 ? 'long' : 'short', startTime: new Date().toISOString() });
      } else {
          await completePomodoro(activeSession.id, 'abandoned');
      }
      onTaskUpdated();
      clearSession();
      setMode('focus');
      setBreakStartTime(null);
      setIsStopModalOpen(false);
      setStopAction(null);
    } catch (e: any) {}
  };

  const mins = isNaN(timeLeft) ? '00' : Math.floor(timeLeft / 60).toString().padStart(2, '0');
  const secs = isNaN(timeLeft) ? '00' : (timeLeft % 60).toString().padStart(2, '0');

  if (!activeSession && !interSessionBreak) {
      return (
        <div className="relative bg-indigo-50/50 border border-indigo-100 rounded-3xl p-8 flex flex-col items-center justify-center gap-5 w-full min-h-[14rem]">
          <div className="absolute top-6 right-6">
             <label className="flex items-center gap-2 text-xs text-indigo-500 font-bold bg-white px-3 py-1.5 rounded-lg border shadow-sm cursor-pointer">
               <input type="checkbox" checked={isDebugMode} onChange={() => setIsDebugMode(!isDebugMode)} className="w-4 h-4 text-indigo-600 rounded" />
               DEBUG MENU
             </label>
          </div>
          <button onClick={handleGenericStart} className="bg-indigo-600 text-white font-semibold py-3 px-8 rounded-xl transition-all shadow-md hover:-translate-y-0.5">Свободный Помодоро</button>
        </div>
      );
  }

  if (interSessionBreak) {
      return (
        <div className="relative border bg-blue-50/50 border-blue-200 rounded-3xl p-8 flex flex-col items-center gap-6 w-full shadow-sm min-h-[16rem]">
          <div className="absolute top-6 right-6">
             <label className="flex items-center gap-2 text-xs text-blue-500 font-bold bg-white px-3 py-1.5 rounded-lg border shadow-sm cursor-pointer">
               <input type="checkbox" checked={isDebugMode} onChange={() => setIsDebugMode(!isDebugMode)} />DEBUG MENU
             </label>
          </div>
          <div className="mt-8 flex flex-col items-center">
            <div className="text-[5rem] font-mono font-bold text-blue-900">{mins}:{secs}</div>
            <button onClick={() => setInterSessionBreak(null)} className="flex items-center gap-2 bg-white text-blue-700 px-6 py-3 rounded-xl font-bold hover:bg-blue-50">Выйти из перерыва</button>
          </div>
        </div>
      )
  }

  return (
    <>
      <div className="relative border bg-white rounded-3xl p-8 flex flex-col items-center gap-6 shadow-sm min-h-[16rem]">
        <div className="absolute top-6 right-6 z-10">
           <label className="flex items-center gap-1.5 bg-gray-100 px-3 py-1.5 rounded-lg shadow-sm border border-gray-200 font-bold text-xs cursor-pointer">
              <input type="checkbox" checked={isDebugMode} onChange={() => setIsDebugMode(!isDebugMode)} className="w-4 h-4 rounded text-indigo-600"/>DEBUG MENU
           </label>
        </div>
        <div className="text-[5.5rem] font-mono font-black text-gray-900 my-2">{mins}:{secs}</div>
        <div className="flex flex-wrap justify-center gap-4 w-full">
          {mode === 'focus' && <button onClick={handleTakeCoffeeBreak} className="border-2 border-blue-100 text-blue-600 px-6 py-3.5 rounded-xl font-bold">Кофе-брейк</button>}
          <button onClick={() => {setStopAction('completed'); setIsStopModalOpen(true)}} className="bg-green-500 text-white px-8 py-3.5 rounded-xl font-bold">Завершить</button>
        </div>
      </div>

      <Modal isOpen={isStopModalOpen} onClose={() => setIsStopModalOpen(false)} title="Подтверждение">
        <div className="space-y-6">
          <p className="text-gray-600">Завершить?</p>
          <div className="flex gap-3 justify-end pt-4">
            <button onClick={() => setIsStopModalOpen(false)} className="px-6 py-3 rounded-xl font-bold text-gray-500">Отмена</button>
            <button onClick={confirmStop} className="px-8 py-3 rounded-xl font-bold text-white bg-green-500">Да</button>
          </div>
        </div>
      </Modal>
    </>
  );
}

fs.writeFileSync('C:\\Users\\konst\\GolandProjects\\toDoNotificator\\frontend-react\\src\\components\\PomodoroWidget.tsx', content, 'utf8');
