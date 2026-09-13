import React from "react";
import { observer } from "mobx-react-lite";
import { DatePicker, Select, Button } from "antd";
import { todoStore } from "../../stores/TodoStore";
import { useCreateTaskMutation } from "../../hooks/useTodos";
import { useCategories } from "../../hooks/useCategories"; // Тот самый разделенный хук
import "./TaskForm.scss";

const TaskForm: React.FC = observer(() => {
  // Вытаскиваем массив напрямую из обновленного хука
  const { data: categories = [], isLoading: isCatsLoading } = useCategories();
  const { mutate: createTask, isPending } = useCreateTaskMutation();

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createTask(undefined, {
      onSuccess: () => {
        todoStore.resetNewTask();
      },
    });
  };

  return (
    <form className="task-form-container" onSubmit={handleSubmit}>
      <h3 className="form-title">Новая задача</h3>

      <div className="form-row">
        <div className="form-group flex-grow">
          <input
            className="custom-input"
            type="text"
            placeholder="Название задачи *"
            value={todoStore.newTask.title}
            onChange={(e) => todoStore.setNewTaskField("title", e.target.value)}
            required
          />
        </div>

        <div className="form-group">
          <label>Дедлайн</label>
          <DatePicker
            className="custom-picker"
            placeholder="ДД.ММ.ГГГГ --:--"
            showTime
            format="DD.MM.YYYY HH:mm"
            onChange={(date) =>
              todoStore.setNewTaskField(
                "deadline",
                date ? date.toISOString() : "",
              )
            }
          />
        </div>

        <div className="form-group">
          <label>Напомнить</label>
          <DatePicker
            className="custom-picker"
            placeholder="ДД.ММ.ГГГГ --:--"
            showTime
            format="DD.MM.YYYY HH:mm"
            onChange={(date) =>
              todoStore.setNewTaskField(
                "reminder_at",
                date ? date.toISOString() : "",
              )
            }
          />
        </div>
      </div>

      <div className="form-row items-end">
        <div className="form-group flex-grow">
          <textarea
            className="custom-textarea"
            placeholder="Описание (необязательно)"
            value={todoStore.newTask.description}
            onChange={(e) =>
              todoStore.setNewTaskField("description", e.target.value)
            }
          />
        </div>

        <div className="form-group">
          <label>Категория</label>
          <Select
            className="custom-select"
            placeholder="Выберите категорию"
            loading={isCatsLoading}
            // Используем undefined для отображения placeholder вместо null
            value={todoStore.newTask.category_id || undefined}
            onChange={(value) =>
              todoStore.setNewTaskField("category_id", value ? +value : null)
            }
            options={[
              // Используем null здесь, так как в onChange мы его приводим
              { value: null, label: "Без категории" },
              ...categories.map((cat) => ({
                value: cat.id,
                label: cat.name,
              })),
            ]}
          />
        </div>

        <Button
          type="primary"
          htmlType="submit"
          className="add-task-btn"
          loading={isPending}
        >
          + Добавить
        </Button>
      </div>
    </form>
  );
});

export default TaskForm;
