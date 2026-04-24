import React from "react";
import { observer } from "mobx-react-lite";
import { todoStore } from "../../stores/TodoStore";
import { useCreateTaskMutation } from "../../hooks/useTodos";
import "./TaskForm.scss";

const TaskForm: React.FC = observer(() => {
  // Подключаем наш хук мутации
  const { mutate, isPending } = useCreateTaskMutation();

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    // Вызываем мутацию (аргумент undefined, т.к. данные берутся из стора)
    mutate(undefined);
  };

  return (
    <form className="task-form-card" onSubmit={handleSubmit}>
      <header className="form-header">
        <h3>Новая задача</h3>
      </header>

      <div className="form-group">
        <label htmlFor="task-title">Название</label>
        <input
          id="task-title"
          type="text"
          placeholder="Что нужно сделать?"
          // Данные из MobX
          value={todoStore.newTask.title}
          // Тот самый типизированный метод
          onChange={(e) => todoStore.setNewTaskField("title", e.target.value)}
          required
        />
      </div>

      <div className="form-group">
        <label htmlFor="task-desc">Описание</label>
        <textarea
          id="task-desc"
          placeholder="Детали задачи..."
          value={todoStore.newTask.description}
          onChange={(e) =>
            todoStore.setNewTaskField("description", e.target.value)
          }
        />
      </div>

      <button type="submit" className="submit-btn" disabled={isPending}>
        {isPending ? "Добавление..." : "Добавить задачу"}
      </button>
    </form>
  );
});

export default TaskForm;
