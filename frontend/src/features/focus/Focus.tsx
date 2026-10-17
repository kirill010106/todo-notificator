import { useState } from "react";
import { Play, Square, Coffee, Check, ArrowUpRight } from "lucide-react";
import { Link } from "react-router-dom";
import { useApp } from "../../app/context";
import { Button, Heading, Sheet, State } from "../../components/ui";
import { useFocus } from "./FocusProvider";
export {
  FocusProvider,
  useFocus,
  secondsLeft,
  loadActiveSession,
} from "./FocusProvider";
import s from "../../styles/App.module.css";
export function FocusPanel({ full = false }: { full?: boolean }) {
  const focus = useFocus();
  const { data } = useApp();
  const [dialog, setDialog] = useState<"result" | "interrupt" | null>(null);
  const onBreak = focus.phase === "extra-break" || focus.phase === "rest";
  const minutes = String(Math.floor(focus.seconds / 60)).padStart(2, "0");
  const remainder = String(focus.seconds % 60).padStart(2, "0");
  const title = [
    ...(focus.selectedTask ? [focus.selectedTask] : []),
    ...(data.tasks || []),
  ].find((task) => task.id === focus.session?.task_id)?.title;
  const finish = async (done: boolean) => {
    if (await focus.stop(done ? "completed" : "abandoned", true))
      setDialog(null);
  };
  return (
    <section className={`${s.focusCard} ${full ? s.focusFull : ""}`}>
      <div className={s.rowBetween}>
        <span className={s.eyebrow}>ПРОСТРАНСТВО ФОКУСА</span>
        {!full && (
          <Link
            to="/app/focus"
            aria-label="Открыть фокус"
            className={s.iconButton}
          >
            <ArrowUpRight size={18} />
          </Link>
        )}
      </div>
      <div
        className={s.timerRing}
        style={
          {
            "--progress": `${focus.session && !onBreak ? Math.max(0, Math.min(100, (1 - focus.seconds / (focus.session.duration_minutes * 60)) * 100)) : 0}%`,
          } as React.CSSProperties
        }
      >
        <div>
          <span className={s.timer}>
            {minutes}
            <span>:</span>
            {remainder}
          </span>
          <small>
            {onBreak
              ? focus.seconds
                ? "время отдохнуть"
                : "перерыв окончен"
              : focus.session
                ? focus.seconds
                  ? "время для важного"
                  : "завершаем сессию"
                : "одна задача за раз"}
          </small>
        </div>
      </div>
      <h3>
        {onBreak
          ? "Сделай паузу. Ты заслужил."
          : focus.session
            ? title ||
              (focus.session.task_id
                ? `Задача №${focus.session.task_id}`
                : "Свободный фокус")
            : "Меньше шума. Больше дела."}
      </h3>
      <p className={s.muted}>
        {onBreak
          ? "Готов вернуться? Продолжи работу, когда захочешь."
          : focus.session
            ? "Ты уже сделал главное — начал."
            : "Выдели 25 минут для того, что действительно важно."}
      </p>
      {full && focus.phase === "idle" && (
        <div className={s.stack}>
          <span>Над чем поработаем?</span>
          <strong>{focus.selectedTask?.title || "Свободная сессия"}</strong>
          <Link className={s.secondary} to="/app/tasks?pick=focus">
            Выбрать задачу из списка
          </Link>
          {focus.selectedTask && (
            <Button quiet onClick={() => focus.chooseTask(null)}>
              Без задачи
            </Button>
          )}
        </div>
      )}
      {focus.canCancel && (
        <Button quiet disabled={focus.busy} onClick={() => void focus.cancel()}>
          Отменить случайный старт · без штрафа
        </Button>
      )}
      {focus.error && (
        <p className={s.error} role="alert">
          {focus.error} <button onClick={focus.retry}>Обновить сессию</button>
        </p>
      )}
      {onBreak ? (
        <div className={s.stack}>
          <Button disabled={focus.busy} onClick={focus.resume}>
            <Play size={16} />
            Продолжить работу
          </Button>
          {focus.phase === "rest" && (
            <Button quiet disabled={focus.busy} onClick={focus.finishRest}>
              Закончить отдых
            </Button>
          )}
        </div>
      ) : !focus.session ? (
        <Button disabled={focus.busy} onClick={() => focus.requestStart()}>
          <Play size={16} />
          Начать фокус
        </Button>
      ) : (
        <div className={s.stack}>
          <Button
            disabled={focus.busy || focus.seconds === 0}
            onClick={() => {
              if (focus.session?.task_id) setDialog("result");
              else {
                const eligibleMinutes = (focus.session?.duration_minutes || 25) - 24;
                void focus.stop(
                  focus.seconds <= Math.max(0, eligibleMinutes * 60)
                    ? "completed"
                    : "abandoned",
                  false,
                );
              }
            }}
          >
            <Square size={16} />
            Закончить
          </Button>
          {!focus.session.breaks_used && (
            <Button
              quiet
              disabled={focus.busy || focus.seconds === 0}
              onClick={() => void focus.pause()}
            >
              <Coffee size={16} />
              Перерыв · 5 минут
            </Button>
          )}
        </div>
      )}
      <div className={s.focusFoot}>
        <span className={s.dot} />
        {focus.phase === "extra-break"
          ? "Один дополнительный перерыв за сессию"
          : focus.phase === "rest"
            ? "Следующий шаг — в твоём темпе"
            : focus.session
              ? "Сессия сохраняется при переходах"
              : "Фокус начинается с маленького шага"}
      </div>
      <Sheet
        open={!!dialog}
        onClose={() => {
          if (!focus.busy) setDialog(null);
        }}
        title={
          dialog === "interrupt" ? "Нужна передышка?" : "Задача уже выполнена?"
        }
        description={
          dialog === "interrupt"
            ? "Можно взять один перерыв на 5 минут и вернуться к той же сессии."
            : "Если всё готово — отметь задачу выполненной. Иначе продолжи фокус или сделай паузу."
        }
        wide
      >
        <div className={s.stack}>
          {focus.error && (
            <p className={s.error} role="alert">
              {focus.error}
            </p>
          )}
          {dialog === "result" ? (
            <>
              <Button disabled={focus.busy} onClick={() => void finish(true)}>
                <Check size={16} />
                Да, задача выполнена
              </Button>
              <Button
                quiet
                disabled={focus.busy}
                onClick={async () => {
                  if (await focus.stop("completed", false)) setDialog(null);
                }}
              >
                Завершить фокус, оставить задачу в работе
              </Button>
              <Button
                quiet
                disabled={focus.busy}
                onClick={() => setDialog("interrupt")}
              >
                Нет, хочу прерваться
              </Button>
              <Button
                quiet
                disabled={focus.busy}
                onClick={() => setDialog(null)}
              >
                Продолжить фокус
              </Button>
            </>
          ) : (
            <>
              {!focus.session?.breaks_used && (
                <Button
                  disabled={focus.busy}
                  onClick={async () => {
                    if (await focus.pause()) setDialog(null);
                  }}
                >
                  <Coffee size={16} />
                  Взять перерыв · 5 минут
                </Button>
              )}
              <p className={s.muted}>
                Если отказаться от задачи, она будет сожжена: −30 XP и сброс
                серии.
              </p>
              <Button
                quiet
                className={s.danger}
                disabled={focus.busy}
                onClick={() => void finish(false)}
              >
                Сдаться и сжечь задачу
              </Button>
              <Button
                quiet
                disabled={focus.busy}
                onClick={() => setDialog(null)}
              >
                Вернуться к фокусу
              </Button>
            </>
          )}
        </div>
      </Sheet>
    </section>
  );
}
export function FocusPage() {
  return (
    <div className={s.focusPage}>
      <Heading eyebrow="ОДНО ДЕЛО В ОДИН МОМЕНТ" title="Время для фокуса">
        Освободи внимание для самого важного.
      </Heading>
      <FocusPanel full />
      <State title="Работай в своём ритме">
        25 минут работы, затем 5 минут отдыха. После четырёх сессий — 15 минут.
        Во время дополнительного перерыва серверное время сессии продолжает
        идти.
      </State>
    </div>
  );
}
