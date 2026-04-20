import TaskItem from "../TaskItem/TaskItem";
import "./TaskList.scss";

interface TaskListProps {
  tasks: any[];
  loading: boolean;
  onToggle: (id: number, currentStatus: string) => void;
  onDelete: (id: number) => void;
}

const TaskList: React.FC<TaskListProps> = ({
  tasks,
  loading,
  onToggle,
  onDelete,
}) => {
  if (loading) return <div className="loading-state">Загрузка задач...</div>;

  return (
    <div className="task-list-container">
      {tasks.length === 0 ? (
        <div className="empty-state">Задач пока нет. Отдохните! ☕</div>
      ) : (
        tasks.map((task) => (
          <TaskItem
            key={task.id}
            task={task}
            onToggle={onToggle}
            onDelete={onDelete}
          />
        ))
      )}
    </div>
  );
};

export default TaskList;
