import React from "react";
import "./TaskHeader.scss";

interface Stats {
  total: number;
  done: number;
  pending: number;
}

interface TaskHeaderProps {
  stats: Stats;
  onLogout: () => void;
}

const TaskHeader: React.FC<TaskHeaderProps> = ({ stats, onLogout }) => {
  return (
    <header className="task-header">
      <div className="header-main">
        <div className="user-info">
          <div className="avatar">🚀</div>
          <div>
            <h1>Мои задачи</h1>
            <p>Добро пожаловать в ToDo Notificator</p>
          </div>
        </div>

        <button className="logout-btn" onClick={onLogout}>
          Выйти 🚪
        </button>
      </div>

      <div className="stats-bar">
        <div className="stat-card">
          <span className="stat-value">{stats.total}</span>
          <span className="stat-label">Всего</span>
        </div>
        <div className="stat-card">
          <span className="stat-value">{stats.pending}</span>
          <span className="stat-label">В процессе</span>
        </div>
        <div className="stat-card success">
          <span className="stat-value">{stats.done}</span>
          <span className="stat-label">Завершено</span>
        </div>
      </div>
    </header>
  );
};

export default TaskHeader;
