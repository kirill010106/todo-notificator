import {
  act,
  fireEvent,
  render,
  screen,
  cleanup,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FocusProvider, FocusPanel, useFocus } from "./Focus";
import { api, ApiError } from "../../lib/api";
import { useApp } from "../../app/context";
import type { Session, FocusSnapshot } from "../../lib/types";
vi.mock("../../app/context", () => ({ useApp: vi.fn() }));
vi.mock("../../lib/api", async (original) => ({
  ...(await original<typeof import("../../lib/api")>()),
  api: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
function Probe() {
  const focus = useFocus();
  return (
    <output data-testid="clock">
      {focus.phase}:{focus.seconds}
    </output>
  );
}
function setup(elapsed?: number, failStop = false, count = 0) {
  vi.mocked(api).mockReset();
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-14T12:00:00Z"));
  let session: Session | null =
    elapsed === undefined
      ? null
      : {
          id: 9,
          task_id: 1,
          started_at: new Date(Date.now() - elapsed * 1000).toISOString(),
          duration_minutes: 25,
          breaks_used: 0,
          status: "active",
        };
  let breakUntil = 0;
  let phase: FocusSnapshot["phase"] = session ? "focus" : "idle";
  const snapshot = (): FocusSnapshot => ({
    server_now: new Date().toISOString(),
    phase,
    session,
    phase_ends_at: breakUntil
      ? new Date(breakUntil).toISOString()
      : session
        ? new Date(Date.parse(session.started_at) + 1500000).toISOString()
        : null,
    remaining_work_seconds: session
      ? Math.min(
          1500,
          Math.max(
            0,
            Math.ceil(
              (Date.parse(session.started_at) + 1500000 - Date.now()) / 1000,
            ),
          ),
        )
      : 0,
    completed_in_cycle: count,
    available_actions: [],
  });
  vi.mocked(useApp).mockImplementation(
    () =>
      ({
        data: {
          user: { id: 1 },
          focus: snapshot(),
          tasks: [{ id: 1, title: "Важная задача" }],
        },
        notify: vi.fn(),
        refresh: vi.fn(async () => {}),
        award: vi.fn(),
      }) as unknown as ReturnType<typeof useApp>,
  );
  vi.mocked(api).mockImplementation(async (path, init) => {
    if (path === "/me/focus") return { focus: snapshot() } as never;
    if (path === "/pomodoros/start") {
      session = {
        id: 10,
        task_id: 1,
        started_at: new Date(Date.now() + 500).toISOString(),
        duration_minutes: 25,
        status: "active",
      };
      phase = "focus";
    } else if (path.endsWith("/pause")) {
      session!.breaks_used = 1;
      phase = "extra_break";
      breakUntil = Date.now() + 300000;
    } else if (path === "/me/focus/resume") {
      phase = "focus";
      breakUntil = 0;
    } else if (path.endsWith("/stop")) {
      if (failStop) throw new ApiError(500, "Не удалось завершить сессию");
      expect(JSON.parse(init!.body as string)).toHaveProperty("action");
      session = null;
      phase = "recovery_break";
      count = (count + 1) % 4;
      breakUntil = Date.now() + (count === 0 ? 900000 : 300000);
    } else throw new Error(`Unexpected ${path}`);
    return {
      focus: snapshot(),
      result: { pomodoro_counted: false },
      rewards: {
        task_xp: 0,
        pomodoro_xp: 0,
        achievement_xp: 0,
        quest_xp: 0,
        penalty_xp: 0,
      },
    } as never;
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const mount = () =>
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <FocusProvider>
            <FocusPanel />
            <Probe />
          </FocusProvider>
        </MemoryRouter>
      </QueryClientProvider>,
    );
  return { client, mount };
}
describe("server Pomodoro flow", () => {
  it("stays idle after refreshing following task creation", async () => {
    const { client, mount } = setup();
    mount();
    await act(async () => {
      await client.invalidateQueries({ queryKey: ["focus"] });
    });
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.getByRole("button", { name: "Начать фокус" })).toBeVisible();
  });
  it("starts at 25:00 with a newer server timestamp", async () => {
    const { mount } = setup();
    mount();
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Начать фокус" }));
    });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Начать сессию" }));
    });
    expect(screen.getByTestId("clock")).toHaveTextContent("focus:1500");
  });
  it("resumes the same session from a server-stored extra break", async () => {
    const { mount } = setup(60);
    mount();
    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: "Перерыв · 5 минут" }),
      );
    });
    expect(screen.getByTestId("clock")).toHaveTextContent("extra-break:300");
    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: "Продолжить работу" }),
      );
    });
    expect(screen.getByTestId("clock")).toHaveTextContent("focus:1440");
    expect(
      vi.mocked(api).mock.calls.some(([path]) => path === "/pomodoros/start"),
    ).toBe(false);
  });
  it("asks for the task outcome before an interruption", () => {
    const { mount } = setup(60);
    mount();
    fireEvent.click(screen.getByRole("button", { name: "Закончить" }));
    expect(
      screen.getByRole("heading", { name: "Задача уже выполнена?" }),
    ).toBeVisible();
    fireEvent.click(
      screen.getByRole("button", { name: "Нет, хочу прерваться" }),
    );
    expect(
      screen.getByRole("heading", { name: "Нужна передышка?" }),
    ).toBeVisible();
  });
  for (const count of [0, 3])
    it(`uses the server recovery break after session ${count + 1}`, async () => {
      const { mount } = setup(1499, false, count);
      mount();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1500);
      });
      expect(screen.getByTestId("clock")).toHaveTextContent(
        `rest:${count === 3 ? 900 : 300}`,
      );
      expect(api).toHaveBeenCalledWith(
        "/pomodoros/9/stop",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ action: "completed", finish_task: false }),
        }),
      );
    });
  it("does not retry failed automatic completion in a loop", async () => {
    const { mount } = setup(1499, true);
    mount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Сервер временно не смог выполнить действие",
    );
    expect(
      vi.mocked(api).mock.calls.filter(([path]) => path.endsWith("/stop")),
    ).toHaveLength(1);
  });
});
