import { makeAutoObservable, runInAction } from "mobx";

class PomodoroStore {
  session: any | null = null;
  isActive: boolean = false;
  timeLeft: number = 0;
  mode: "focus" | "break" = "focus";
  timerId: ReturnType<typeof setInterval> | null = null;

  constructor() {
    // Используем autoBind, чтобы не терять контекст this в setInterval
    makeAutoObservable(this, {}, { autoBind: true });
  }

  setSession(session: any) {
    this.session = session;
  }

  start(seconds: number) {
    console.log("Store: Starting timer with", seconds, "seconds");
    this.isActive = true;
    this.timeLeft = seconds;

    if (this.timerId) clearInterval(this.timerId);

    this.timerId = setInterval(() => {
      // Все изменения состояния внутри асинхронных функций — только через runInAction
      runInAction(() => {
        if (this.timeLeft > 0) {
          this.timeLeft--;
        } else {
          this.stop();
        }
      });
    }, 1000);
  }

  stop() {
    console.log("Store: Stopping timer");
    this.isActive = false;
    this.session = null;
    this.timeLeft = 0;
    if (this.timerId) {
      clearInterval(this.timerId);
      this.timerId = null;
    }
  }

  get formatTime() {
    const mins = Math.floor(this.timeLeft / 60);
    const secs = this.timeLeft % 60;
    return `${mins}:${secs.toString().padStart(2, "0")}`;
  }
}

export const pomodoroStore = new PomodoroStore();
