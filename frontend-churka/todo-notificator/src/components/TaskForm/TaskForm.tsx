import React, { useState } from "react";
import "./TaskForm.scss";

interface TaskFormProps {
  onTaskCreated: () => void;
}

const TaskForm: React.FC<TaskFormProps> = ({ onTaskCreated }) => {
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (isSubmitting) return;

    setIsSubmitting(true);
    const token = localStorage.getItem("token");

    try {
      const res = await fetch("http://localhost:8082/api/v1/tasks", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          title,
          description,
          status: "pending",
        }),
      });

      if (res.ok) {
        setTitle("");
        setDescription("");
        onTaskCreated();
      }
    } catch (err) {
      console.error("Failed to create task:", err);
    } finally {
      setIsSubmitting(false);
    }
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
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          required
        />
      </div>

      <div className="form-group">
        <label htmlFor="task-desc">Описание</label>
        <textarea
          id="task-desc"
          placeholder="Детали задачи..."
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </div>

      <button type="submit" className="submit-btn" disabled={isSubmitting}>
        {isSubmitting ? "Добавление..." : "Добавить задачу"}
      </button>
    </form>
  );
};

export default TaskForm;
