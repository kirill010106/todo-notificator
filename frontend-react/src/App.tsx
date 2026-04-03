import { useEffect, useState } from 'react'
import { Login } from './components/Login'
import { useAuth } from './store/useAuth'
import { PomodoroWidget } from './components/PomodoroWidget'
import { StatsWidget } from './components/StatsWidget'
import { CategoriesWidget } from './components/CategoriesWidget'
import { NewTaskWidget } from './components/NewTaskWidget'
import { TaskItem } from './components/TaskItem'
import { getTasks, type Task } from './api/tasks'
import { ToastContainer } from './components/ui/ToastContainer'

function App() {
  const { accessToken, logout } = useAuth()
  const [tasks, setTasks] = useState<Task[]>([])

  const fetchTasks = async () => {
    try {
      const data = await getTasks()
      setTasks(data || [])
    } catch (e) {
      console.error(e)
    }
  }

  useEffect(() => {
    if (accessToken) {
      fetchTasks()
    }
  }, [accessToken])

  if (!accessToken) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center p-8">
        <Login />
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-gray-50/50 text-gray-900 p-4 md:p-8 selection:bg-indigo-100 selection:text-indigo-900">
      <div className="max-w-[1400px] mx-auto flex flex-col gap-10">
        <div className="flex justify-between items-center bg-white px-8 py-5 rounded-3xl shadow-sm border border-gray-100">
          <div className="flex items-center gap-3">
             <div className="bg-indigo-50 p-2 rounded-2xl text-2xl">🍅</div>
             <h1 className="text-2xl font-black tracking-tight text-gray-900">ToDo Notificator</h1>
          </div>
          <button 
             onClick={logout} 
             className="px-5 py-2.5 rounded-xl font-semibold text-gray-500 hover:bg-red-50 hover:text-red-500 transition-colors"
          >
             Выйти
          </button>
        </div>

        <div className="grid grid-cols-1 xl:grid-cols-12 gap-8 items-start">     
          
          <div className="xl:col-span-8 flex flex-col gap-8">
            <PomodoroWidget tasks={tasks} onTaskUpdated={fetchTasks} />
            <NewTaskWidget onTaskCreated={fetchTasks} />

            <div className="bg-white shadow-sm border border-gray-200 p-8 rounded-3xl">
              <h2 className="text-2xl font-bold mb-6 flex items-center gap-2">
                 <span className="text-indigo-500">❖</span> Ваши задачи
                 <span className="ml-auto text-sm font-semibold bg-gray-100 text-gray-500 px-3 py-1 rounded-full">{tasks.length} всего</span>
              </h2>
              
              <div className="flex flex-col gap-4">
                {!tasks || tasks.length === 0 ? (
                  <div className="p-8 text-center border-2 border-dashed border-gray-200 rounded-3xl text-gray-400 font-medium">Нет активных задач. Пора создать новую!</div>
                ) : (
                  tasks.map(task => (
                    <TaskItem key={task.id} task={task} onUpdate={fetchTasks} />
                  ))
                )}
              </div>
            </div>
          </div>

          <div className="xl:col-span-4 flex flex-col gap-8 sticky top-8">
            <StatsWidget />
            <CategoriesWidget />
          </div>

        </div>
      </div>
      
      {/* Global Toast Container added to App */}
      <ToastContainer />
    </div>
  )
}

export default App