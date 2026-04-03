import { useState, useEffect } from 'react';
import { usePomodoro } from '../store/usePomodoro';
import { pausePomodoro, completePomodoro, startPomodoro } from '../api/pomodoro';
import { differenceInSeconds } from 'date-fns';
import { Modal } from './ui/Modal';
import { useToast } from '../store/useToast';
import { type Task } from '../api/tasks';
import { Play } from 'lucide-react';

export function PomodoroWidget({ onTaskUpdated }: { tasks: Task[], onTaskUpdated: () => void }) {
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
  const [isDebugMode, setIsDebugMode] = useState(false);

  const cycleBoxes = [1, 2, 3, 4];
  const currentCycleProgress = completedPomodorosCount % 4;

  useEffect(() => {
    let interval: ReturnType<typeof setInterval>;
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
        if (e.response?.status === 409 && e.response.data?.active_session) {
            if (window.confirm('У вас уже есть активный таймер. Продолжить его?')) {
                setActiveSession(e.response.data.active_session);
                setMode('focus');
                setBreakStartTime(null);
                setInterSessionBreak(null);
            }
        } else {
            addToast('Ошибка запуска', 'error');
        }
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

  const handleReturnToWork = () => {
      setMode('focus');
      setBreakStartTime(null);
      addToast('Возвращаемся к работе', 'success');
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
             <label className="flex items-center gap-2 text-xs text-indigo-500 font-bold bg-white px-3 py-1.5 rounded-lg border shadow-sm cursor-pointer hover:bg-slate-50 transition-colors">
               <input type="checkbox" checked={isDebugMode} onChange={() => setIsDebugMode(!isDebugMode)} className="w-4 h-4 text-indigo-600 border-indigo-200 rounded cursor-pointer" />
               DEBUG MENU
             </label>
          </div>
          <div className="bg-indigo-100 p-4 rounded-full text-indigo-500">
            <Play size={32} fill="currentColor" className="ml-1" />
          </div>
          <div className="flex flex-col items-center">
            <h3 className="text-xl font-bold text-indigo-900 mb-1">Готовы сфокусироваться?</h3>
            <button onClick={handleGenericStart} className="bg-indigo-600 text-white font-semibold py-3 px-8 rounded-xl transition-all shadow-md hover:-translate-y-0.5 mt-4">Свободный Помодоро</button>
            <div className="mt-4 flex gap-2 justify-center">
                {cycleBoxes.map(i => (
                    <div key={i} className={`w-3 h-3 rounded-full ${i <= currentCycleProgress ? 'bg-indigo-500' : 'bg-indigo-200'}`} title="Прогресс"></div>
                ))}
            </div>
            {completedPomodorosCount > 0 && (
                <div className="text-[10px] text-indigo-400 mt-2 font-medium uppercase tracking-wider">Всего завершено: {completedPomodorosCount} 🍅</div>
            )}
          </div>
        </div>
      );
  }

  if (interSessionBreak) {
      return (
        <div className="relative border bg-blue-50/50 border-blue-200 rounded-3xl p-8 flex flex-col items-center gap-6 w-full shadow-sm min-h-[16rem]">
          <div className="absolute top-6 right-6">
             <label className="flex items-center gap-2 text-xs text-blue-500 font-bold bg-white px-3 py-1.5 rounded-lg border shadow-sm cursor-pointer hover:bg-slate-50 transition-colors">
               <input type="checkbox" checked={isDebugMode} onChange={() => setIsDebugMode(!isDebugMode)} className="w-4 h-4 text-indigo-600 border-indigo-200 rounded cursor-pointer" />
               DEBUG MENU
             </label>
          </div>
          <div className="mt-8 flex flex-col items-center">
            <h3 className="text-blue-800 text-xl font-bold mb-4">{interSessionBreak.type === 'long' ? 'Длинный перерыв' : 'Короткий перерыв'}</h3>
            <div className="text-[5rem] font-mono font-bold text-blue-900 animate-pulse">{mins}:{secs}</div>
            <button onClick={() => setInterSessionBreak(null)} className="flex items-center gap-2 mt-4 bg-white text-blue-700 px-6 py-3 rounded-xl font-bold hover:bg-blue-50 shadow-sm border border-blue-100">Выйти из перерыва</button>
          </div>
        </div>
      )
  }

  return (
    <>
      <div className="relative border bg-white rounded-3xl p-8 flex flex-col items-center gap-6 shadow-sm min-h-[16rem]">
        <div className="absolute top-6 right-6 z-10">
           <label className="flex items-center gap-1.5 bg-gray-100 px-3 py-1.5 rounded-lg shadow-sm border border-gray-200 font-bold text-xs cursor-pointer hover:bg-gray-200 transition-colors text-indigo-600">
              <input type="checkbox" checked={isDebugMode} onChange={() => setIsDebugMode(!isDebugMode)} className="w-4 h-4 rounded text-indigo-600 border-indigo-300 cursor-pointer"/>
              DEBUG MENU
           </label>
        </div>
        

        <div className="absolute top-6 left-6 z-10 flex flex-col gap-2">
            <div className="flex items-center gap-2">
                <div className={`w-3 h-3 rounded-full animate-pulse ${mode === 'focus' ? 'bg-indigo-500' : 'bg-blue-403'}`} />
                <span className={`text-sm font-semibold uppercase tracking-wider ${mode === 'focus' ? 'text-indigo-600' : 'text-blue-600'}`}>
                   {mode === 'focus' ? 'focus' : 'Break'}
                </span>
            </div>
            <div className="flex gap-1.5 ml-1 mt-1">
                {cycleBoxes.map(i => (
                    <div key={i} className={`w-2 h-2 rounded-full ${i <= currentCycleProgress ? 'bg-indigo-500' : 'bg-indigo-100'}`} title="Прогресс до длинного перерыва"/>
                ))}
            </div>
        </div>

        
        <div className="text-[5.5rem] font-mono font-black text-gray-900 mt-6 mb-2">{mins}:{secs}</div>
        <div className="flex flex-wrap justify-center gap-4 w-full">
          {mode === 'focus' ? (
              <button onClick={handleTakeCoffeeBreak} className="border-2 border-blue-100 bg-blue-50 text-blue-600 hover:bg-blue-100 px-6 py-3.5 rounded-xl font-bold">Кофе-брейк</button>
          ) : (
              <button onClick={handleReturnToWork} className="border-2 border-blue-100 bg-blue-50 text-blue-600 hover:bg-blue-100 px-6 py-3.5 rounded-xl font-bold">Вернуться к работе</button>
          )}
          <button onClick={() => {setStopAction('completed'); setIsStopModalOpen(true)}} className="bg-green-500 hover:bg-green-600 text-white px-8 py-3.5 rounded-xl font-bold shadow-sm shadow-green-200">Завершить</button>
          <button onClick={() => {setStopAction('abandoned'); setIsStopModalOpen(true)}} className="border-2 border-red-100 bg-red-50 hover:bg-red-100 text-red-600 px-6 py-3.5 rounded-xl font-bold">Прервать</button>
        </div>
      </div>

      <Modal isOpen={isStopModalOpen} onClose={() => setIsStopModalOpen(false)} title={stopAction === 'completed' ? 'Подтверждение' : 'Прерывание'}>
        <div className="space-y-6">
          <p className="text-gray-600">Вы уверены?</p>
          <div className="flex gap-3 justify-end pt-4">
            <button onClick={() => setIsStopModalOpen(false)} className="px-6 py-3 rounded-xl font-bold text-gray-500 bg-gray-50 hover:bg-gray-100">Отмена</button>
            <button onClick={confirmStop} className={`px-8 py-3 rounded-xl font-bold text-white shadow-sm ${stopAction === 'completed' ? 'bg-green-500 hover:bg-green-600' : 'bg-red-500 hover:bg-red-600'}`}>Да</button>
          </div>
        </div>
      </Modal>
    </>
  );
}
