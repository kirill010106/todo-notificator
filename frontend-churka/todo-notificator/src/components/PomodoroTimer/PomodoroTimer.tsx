import React from "react";
import "./PomodoroTimer.scss";
import { usePomodoro } from "../../hooks/usePomodoro";

const PomodoroTimer: React.FC = () => {
  // Добавляем isLoading и stopSession из хука
  const { startSession, stopSession, isActive, formatTime, mode, isLoading } =
    usePomodoro();

  return (
    <section className="pomodoro">
      <div className="pomodoro__card">
        <div className="pomodoro__inner">
          <div className="pomodoro__icon-wrapper">
            <span role="img" aria-label="tomato">
              🍅
            </span>
          </div>

          {!isActive ? (
            <div className="pomodoro__setup">
              <h3 className="pomodoro__heading">Готовы сфокусироваться?</h3>
              <button
                onClick={() => startSession()}
                disabled={isLoading}
                className="pomodoro__btn-start"
              >
                {isLoading ? "Загрузка..." : "Свободный Помодоро"}
              </button>
              <p className="pomodoro__hint">
                Или нажмите <span>🍅</span> на конкретной задаче ниже, чтобы
                запустить жесткий режим.
              </p>
            </div>
          ) : (
            <div className="pomodoro__active">
              <div className={`pomodoro__timer pomodoro__timer--${mode}`}>
                {formatTime()}
              </div>
              <p className="pomodoro__status">
                {mode === "focus" ? "Время концентрироваться" : "Перерыв"}
              </p>

              {/* Кнопка остановки из макета */}
              <button
                onClick={() => stopSession()}
                disabled={isLoading}
                className="pomodoro__btn-stop"
              >
                Остановить
              </button>
            </div>
          )}
        </div>
      </div>
    </section>
  );
};

export default PomodoroTimer;
