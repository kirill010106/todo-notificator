import React from "react";
import { observer } from "mobx-react-lite";
import { pomodoroStore } from "../../stores/PomodoroStore";
import { usePomodoro } from "../../hooks/usePomodoro";
import "./PomodoroTimer.scss";

const PomodoroTimer: React.FC = observer(() => {
  // Достаем только методы управления и состояние загрузки мутаций
  const { startSession, stopSession, isLoading } = usePomodoro();

  // Основной источник правды — наш MobX стор
  const { isActive, formatTime, mode } = pomodoroStore;

  console.log(isActive);

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
                запустить режим фокуса.
              </p>
            </div>
          ) : (
            <div className="pomodoro__active">
              {/* Таймер меняет цвет в зависимости от режима: focus или break */}
              <div className={`pomodoro__timer pomodoro__timer--${mode}`}>
                {formatTime}
              </div>
              <p className="pomodoro__status">
                {mode === "focus" ? "Время концентрироваться" : "Перерыв"}
              </p>

              <button
                onClick={() => stopSession()}
                disabled={isLoading}
                className="pomodoro__btn-stop"
              >
                {isLoading ? "..." : "Остановить"}
              </button>
            </div>
          )}
        </div>
      </div>
    </section>
  );
});

export default PomodoroTimer;
