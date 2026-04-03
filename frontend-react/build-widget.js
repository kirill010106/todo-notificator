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
    activeSession, 
    setActiveSession, 
    takeBreak, 
    clearSession, 
    completedPomodorosCount, 
    incrementCompleted, 
    resetCompleted,
    interSessionBreak,
    setInterSessionBreak
  } = usePomodoro();

  const [timeLeft, setTimeLeft] = useState<number>(0);
  const [mode, setMode] = useState<'focus' | 'break'>('focus');
  const [breakStartTime, setBreakStartTime] = useState<Date | null>(null);
  
  const { addToast } = useToast();

  const [isStopModalOpen, setIsStopModalOpen] = useState(false);
  const [stopAction, setStopAction] = useState<'completed' | 'abandoned' | null>(null);
  const [markTaskComplete, setMarkTaskComplete] = useState(true);

  // Debug states
  const [isDebugMode, setIsDebugMode] = useState(false);

  const focusedTask = activeSession?.task_id ? tasks.find(t => t.id === activeSession.task_id) : null;

  // Render cycles (0 to 3)
  const cycleBoxes = [1, 2, 3, 4];
  const currentCycleProgress = completedPomodorosCount % 4;

  useEffect(() => {
    let interval: NodeJS.Timeout;

    // State 1: We are in an active session (Focus or Coffee Break)
    if (activeSession) {
      const startTime = mode === 'focus' ? new Date(activeSession.started_at) : (breakStartTime || new Date());
      if (mode === 'break' && !breakStartTime) {
        setBreakStartTime(startTime);
      }

      const focusMinutes = activeSession.duration_minutes;
      const breakMinutes = 5;

      const durationSec = isDebugMode
        ? (mode === 'focus' ? 10 : 5) 
        : (mode === 'focus' ? focusMinutes * 60 : breakMinutes * 60);

      interval = setInterval(() => {
        const elapsed = differenceInSeconds(new Date(), startTime);
        let remaining = Math.max(durationSec - elapsed, 0);
        if (Number.isNaN(remaining)) remaining = 0;

        setTimeLeft(remaining);

        if (remaining === 0) {
          if (mode === 'break') {
            // End of short coffee break. Return to focus!
            setMode('focus');
            setBreakStartTime(null);
            addToast('Кофе-брейк окончен! Возвращаемся к работе.', 'success');
          } else {
            clearInterval(interval);
            addToast('Время фокуса вышло! Пора завершить задачу.', 'info');
          }
        }
      }, 1000);
    } 
    // State 2: We are in an inter-session break (Short or Long Break)
    else if (interSessionBreak) {
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
        setTimeout(() => document.getElementById('pomodoro-widget')?.scrollIntoView({ behavior: 'smooth' }), 100);
    } catch (e: any) {
        if (e.response?.status === 409 && e.response.data?.active_session) {
            if (window.confirm('У вас уже есть активный таймер. Продолжить его?')) {
                setActiveSession(e.response.data.active_session);
                setMode('focus');
                setBreakStartTime(null);
                setInterSessionBreak(null);
                setTimeout(() => document.getElementById('pomodoro-widget')?.scrollIntoView({ behavior: 'smooth' }), 100);
            }
        } else {
            addToast(e.response?.data?.error || 'Ошибка запуска таймера', 'error');
        }
    }
  };

  const handleTakeCoffeeBreak = async () => {
    try {
      const { session } = await pausePomodoro(activeSession!.id);
      takeBreak(session);
      setMode('break');
      addToast('Время отдохнуть 5 минут ☕', 'info');
    } catch (e: any) {
      addToast(e.response?.data?.error || 'Ошибка при взятии перерыва', 'error');
    }
  };

  const promptStop = (action: 'completed' | 'abandoned') => {
    setStopAction(action);
    setMarkTaskComplete(action === 'completed');
    setIsStopModalOpen(true);
  };

  const confirmStop = async () => {
    if (!stopAction || !activeSession) return;

    try {
      if (stopAction === 'completed') {
          await completePomodoro(activeSession.id, 'completed');
          incrementCompleted(); 
          const newTotal = completedPomodorosCount + 1;
          const isLongBreak = newTotal % 4 === 0;
          setInterSessionBreak({
              type: isLongBreak ? 'long' : 'short',
              startTime: new Date().toISOString()
          });
      } else {
          await completePomodoro(activeSession.id, 'abandoned');
      }

      if (stopAction === 'completed' && markTaskComplete && activeSession.task_id) {
         try {
           await updateTask(activeSession.task_id, { status: 'done' });
         } catch (err) {
           console.error('Failed to complete task', err);
         }
      } else if (stopAction === 'abandoned' && activeSession.task_id) {
         try {
           await updateTask(activeSession.task_id, { status: 'burnt' });
         } catch (err) {
           console.error('Failed to finish task as burnt', err);
         }
      }

      onTaskUpdated();
      addToast(stopAction === 'completed' ? 'Сессия успешно завершена 🎉' : 'Сессия прервана 🛑', 'info');

      clearSession();
      setMode('focus');
      setBreakStartTime(null);
      setIsStopModalOpen(false);
      setStopAction(null);
    } catch (e: any) {
      addToast(e.response?.data?.error || 'Ошибка при завершении', 'error');
      setIsStopModalOpen(false);
    }
  };

  const mins = isNaN(timeLeft) ? '00' : Math.floor(timeLeft / 60).toString().padStart(2, '0');
  const secs = isNaN(timeLeft) ? '00' : (timeLeft % 60).toString().padStart(2, '0');

  // Widget structure deciding logic
  if (!activeSession && !interSessionBreak) {
      return (
        <div id="pomodoro-widget" className="relative bg-indigo-50/50 border border-indigo-100 rounded-3xl p-8 flex flex-col items-center justify-center gap-5 w-full min-h-[14rem] transition-all hover:bg-indigo-50">
          
          <div className="absolute top-6 right-6">
             <label className="flex items-center gap-2 text-xs text-indigo-400 font-medium cursor-pointer" title="Быстрые таймеры для теста">
               <input type="checkbox" checked={isDebugMode} onChange={() => setIsDebugMode(!isDebugMode)} className="rounded text-indigo-500 cursor-pointer" />
               Debug
             </label>
          </div>

          <div className="bg-indigo-100 p-4 rounded-full text-indigo-500">
            <Play size={32} fill="currentColor" className="ml-1" />
          </div>
          <div className="flex flex-col items-center">
            <h3 className="text-xl font-bold text-indigo-900 mb-1">Готовы сфокусироваться?</h3>
            <p className="text-indigo-600/70 text-sm mb-4">Начните таймер для конкретной задачи, или свободный здесь.</p>
            <button onClick={handleGenericStart} className="bg-indigo-600 hover:bg-indigo-700 text-white font-semibold py-3 px-8 rounded-xl transition-all shadow-md hover:shadow-lg hover:-translate-y-0.5">
                Свободный Помодоро
            </button>
            <div className="mt-4 flex gap-2 justify-center">
                {cycleBoxes.map(i => (
                   <div key={i} className={\w-3 h-3 rounded-full \\} title="Прогресс"></div>
                ))}
            </div>
            {completedPomodorosCount > 0 && (
                <div className="text-[10px] text-indigo-400 mt-2 font-medium uppercase tracking-wider">Всего завершено: {completedPomodorosCount} 🍅</div>
            )}
          </div>
        </div>
      );
  }

  // Inter-session break widget
  if (interSessionBreak) {
      const isLong = interSessionBreak.type === 'long';
      return (
        <div id="pomodoro-widget" className="relative border bg-blue-50/50 border-blue-200 rounded-3xl p-8 flex flex-col items-center gap-6 w-full shadow-sm transition-all min-h-[16rem]">
          <div className="absolute top-6 left-6 flex items-center gap-2">
              <Coffee size={18} className="text-blue-500" />
              <span className="text-sm font-semibold uppercase tracking-wider text-blue-600">
                {isLong ? 'Длинный перерыв' : 'Короткий перерыв'}
              </span>
          </div>

          <div className="absolute top-6 right-6">
             <label className="flex items-center gap-2 text-xs text-blue-400 font-medium cursor-pointer">
               <input type="checkbox" checked={isDebugMode} onChange={() => setIsDebugMode(!isDebugMode)} className="rounded text-blue-500 cursor-pointer" />
               Debug
             </label>
          </div>

          <div className="mt-8 flex flex-col items-center">
            <div className="text-[5rem] leading-none font-mono font-bold tracking-tight text-blue-900 drop-shadow-sm mb-6">
              {mins}<span className="text-blue-300 animate-pulse">:</span>{secs}
            </div>

            <button
               onClick={() => setInterSessionBreak(null)}
               className="flex items-center gap-2 bg-white border-2 border-blue-200 hover:border-blue-300 text-blue-700 px-6 py-3 rounded-xl font-bold transition-all hover:bg-blue-50"
            >
               <SkipForward size={18} /> Завершить отдых досрочно
            </button>
          </div>
        </div>
      )
  }

  // Active session widget
  return (
    <>
      <div id="pomodoro-widget" className="relative border bg-white rounded-3xl p-8 flex flex-col items-center gap-6 w-full shadow-sm transition-all overflow-hidden min-h-[16rem]">
        
        {/* Progress bar background visualization */}
        {isDebugMode && (
           <div className="absolute top-6 right-6 z-10 flex items-center gap-1.5 bg-gray-100 px-3 py-1.5 rounded-full shadow-sm border border-gray-200">
              <Bug size={14} className="text-gray-500"/> 
              <span className="text-xs font-mono text-gray-600 font-bold uppercase tracking-wider">debug</span>
              <input type="checkbox" checked={isDebugMode} onChange={() => setIsDebugMode(!isDebugMode)} className="ml-1 cursor-pointer w-4 h-4 text-indigo-600 rounded focus:ring-indigo-500"/>
           </div>
        )}
        {!isDebugMode && (
           <div className="absolute top-6 right-6 z-10 opacity-0 hover:opacity-100 transition-opacity flex items-center gap-1.5 bg-gray-50 px-3 py-1.5 rounded-full border border-gray-200 shadow-sm">
              <Bug size={14} className="text-gray-400"/>
              <span className="text-xs font-mono text-gray-500 font-bold uppercase tracking-wider">debug</span>
              <input type="checkbox" checked={isDebugMode} onChange={() => setIsDebugMode(!isDebugMode)} className="ml-1 cursor-pointer w-4 h-4 text-indigo-600 rounded focus:ring-indigo-500"/>
           </div>
        )}

        <div className="absolute top-6 left-6 z-10 flex flex-col gap-2">
            <div className="flex items-center gap-2">
                <div className={\w-3 h-3 rounded-full animate-pulse \\} />
                <span className={\	ext-sm font-semibold uppercase tracking-wider \\}>
                  {mode === 'focus' ? 'Фокус' : 'Кофе-брейк'}
                </span>
            </div>
            {/* Visual cycle dots */}
            <div className="flex gap-1.5 ml-1 mt-1">
                {cycleBoxes.map(i => (
                    <div key={i} className={\w-2 h-2 rounded-full \\} />
                ))}
            </div>
        </div>

        <div className="mt-8 flex flex-col items-center z-10">
            <h3 className="text-3xl font-bold text-gray-900 text-center px-4 leading-tight max-w-lg mb-2">
                {focusedTask ? focusedTask.title : 'Свободная сессия'}
            </h3>
            {focusedTask && (
                 <div className="text-indigo-600/80 font-bold text-xs bg-indigo-100/50 px-3 py-1 rounded-full border border-indigo-200/50 flex items-center justify-center gap-1 shadow-sm">
                     <Flame size={14} className="text-orange-500" /> Активная задача
                 </div>
            )}
        </div>

        <div className="text-[5.5rem] leading-none font-mono font-black tracking-tighter text-gray-900 drop-shadow-sm my-2 z-10 flex">
          {mins}<span className={mode === 'break' ? 'text-blue-300 animate-pulse' : 'text-indigo-300 animate-pulse'}>:</span>{secs}
        </div>

        <div className="flex flex-wrap justify-center gap-4 z-10 w-full mt-2">
          {mode === 'focus' && activeSession?.breaks_used === 0 && (
            <button
              onClick={handleTakeCoffeeBreak}
              className="bg-white border-2 border-blue-100 hover:border-blue-300 text-blue-600 px-6 py-3.5 rounded-xl font-bold transition-all hover:bg-blue-50 focus:ring-4 focus:ring-blue-50 flex items-center gap-2 text-sm uppercase tracking-wide"
            >
              <Coffee size={18} /> Кофе-брейк
            </button>
          )}
          <button
            onClick={() => promptStop('completed')}
            className="flex-1 min-w-[200px] max-w-[280px] bg-green-500 hover:bg-green-600 text-white px-8 py-3.5 rounded-xl shadow-md shadow-green-500/20 font-bold transition-all hover:-translate-y-0.5 focus:ring-4 focus:ring-green-200 flex items-center justify-center gap-2 text-sm uppercase tracking-wide"
          >
            <CheckCircle size={20} /> Завершить
          </button>
          <button
            onClick={() => promptStop('abandoned')}
            className="bg-white border-2 border-red-100 hover:border-red-300 text-red-600 px-6 py-3.5 rounded-xl font-bold transition-all hover:bg-red-50 focus:ring-4 focus:ring-red-50 flex items-center justify-center gap-2 text-sm uppercase tracking-wide"
          >
            <span className="text-lg">🛑</span> Прервать
          </button>
        </div>
      </div>

      <Modal
        isOpen={isStopModalOpen}
        onClose={() => setIsStopModalOpen(false)}
        title={stopAction === 'completed' ? 'Завершение таймера' : 'Прерывание таймера'}
      >
        <div className="space-y-6">
          <p className="text-gray-600 text-lg leading-relaxed">
             {stopAction === 'completed'
               ? 'Вы уверены, что хотите завершить текущую сессию? Потраченные помодоро будут учтены.'
               : 'Вы уверены, что хотите прервать сессию? Прогресс этого таймера не сохранится.'}
          </p>

          {stopAction === 'completed' && focusedTask && (
            <label className="flex items-center gap-3 p-4 border-2 border-indigo-100 bg-indigo-50/40 rounded-xl cursor-pointer hover:bg-indigo-50 transition-colors">
              <input
                type="checkbox"
                className="w-6 h-6 text-indigo-600 rounded border-gray-300 focus:ring-indigo-500 cursor-pointer shadow-sm"
                checked={markTaskComplete}
                onChange={(e) => setMarkTaskComplete(e.target.checked)}
              />
              <span className="text-indigo-900 font-semibold">Отметить задачу "{focusedTask.title}" как выполненную</span>
            </label>
          )}

          <div className="flex gap-3 justify-end pt-4 border-t border-gray-100">
            <button
                onClick={() => setIsStopModalOpen(false)}
                className="px-6 py-3 rounded-xl font-bold text-gray-500 hover:bg-gray-100 transition-colors"
            >
                Отмена
            </button>
            <button
                onClick={confirmStop}
                className={\px-8 py-3 rounded-xl font-bold text-white shadow-sm transition-all hover:-translate-y-0.5 \\}
            >
                {stopAction === 'completed' ? 'Подтвердить' : 'Прервать'}
            </button>
          </div>
        </div>
      </Modal>
    </>
  );
}

fs.writeFileSync('C:\\Users\\konst\\GolandProjects\\toDoNotificator\\frontend-react\\src\\components\\PomodoroWidget.tsx', content, 'utf8');
