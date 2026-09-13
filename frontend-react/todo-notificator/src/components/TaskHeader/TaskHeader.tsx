import React from "react";
import "./TaskHeader.scss";
import { observer } from "mobx-react-lite";

interface Stats {
  points: number;
  level: number;
  current_streak: number;
}

interface TaskHeaderProps {
  stats: Stats;
  onLogout: () => void;
}

const TaskHeader: React.FC<TaskHeaderProps> = observer(
  ({ stats, onLogout }) => {
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
            <span className="stat-value">{stats.points}</span>
            <span className="stat-label">Всего очков</span>
          </div>
          <div className="stat-card">
            <span className="stat-value">{stats.level}</span>
            <span className="stat-label">Уровень</span>
          </div>
          <div className="stat-card success">
            <span className="stat-value">{stats.current_streak}</span>
            <span className="stat-label">Текущая серия</span>
          </div>
        </div>
      </header>
    );
  },
);

export default TaskHeader;
