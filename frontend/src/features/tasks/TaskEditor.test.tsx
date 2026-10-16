import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { TaskEditor } from "./Tasks";
import { api, ApiError } from "../../lib/api";
vi.mock("../../app/context", () => ({
  useApp: () => ({
    data: {
      categories: [],
      user: {
        id: 1,
        is_premium: false,
        is_verified: true,
        active_reminders_count: 0,
        reminders_per_task_limit: 2,
      },
    },
    refresh: vi.fn(),
    notify: vi.fn(),
  }),
}));
vi.mock("../../lib/api", async () => ({
  ...(await vi.importActual<typeof import("../../lib/api")>("../../lib/api")),
  api: vi.fn(),
  errorText: () => "Ошибка сервера",
  json: JSON.stringify,
}));
describe("Task editor", () => {
  it("does not replace reminders when only another task field changed", async () => {
    vi.mocked(api).mockClear();
    vi.mocked(api).mockResolvedValueOnce({});
    render(
      <MemoryRouter>
        <TaskEditor
          open
          close={vi.fn()}
          task={{
            id: 7,
            title: "Старая задача",
            description: "",
            deadline: null,
            reminders: [{ id: 99, offset_minutes: null, scheduled_at: "2026-10-18T12:00:00Z", status: "scheduled", sent_at: null, legacy_absolute: true }],
            status: "pending",
            category_id: null,
          }}
        />
      </MemoryRouter>,
    );
    fireEvent.change(screen.getByLabelText("Название"), { target: { value: "Новое название" } });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить изменения" }));
    await waitFor(() => expect(api).toHaveBeenCalled());
    expect(JSON.parse(vi.mocked(api).mock.calls[0][1]!.body as string)).not.toHaveProperty("reminders");
  });
  it("does not send incomplete dates and focuses the field", async () => {
    vi.mocked(api).mockClear();
    render(
      <MemoryRouter>
        <TaskEditor open task={null} close={vi.fn()} />
      </MemoryRouter>,
    );
    fireEvent.change(screen.getByLabelText("Название"), {
      target: { value: "Черновик" },
    });
    fireEvent.change(screen.getByLabelText("Срок задачи: дата"), {
      target: { value: "18.10.2026" },
    });
    expect(screen.queryByText(/Дата выбрана/)).not.toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Создать задачу" }),
    );
    expect(screen.getByText(/Дата выбрана/)).toBeVisible();
    expect(screen.getByLabelText("Срок задачи: дата")).toHaveFocus();
    expect(screen.getByLabelText("Срок задачи: дата")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(api).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Название")).toHaveValue("Черновик");
  });
  it("rejects impossible and partially typed dates before HTTP", () => {
    vi.mocked(api).mockClear();
    render(
      <MemoryRouter>
        <TaskEditor open task={null} close={vi.fn()} />
      </MemoryRouter>,
    );
    fireEvent.change(screen.getByLabelText("Название"), {
      target: { value: "Задача" },
    });
    for (const date of ["31.02.2099", "18.10.20"]) {
      fireEvent.change(screen.getByLabelText("Срок задачи: дата"), {
        target: { value: date },
      });
      fireEvent.change(screen.getByLabelText("Срок задачи: время"), {
        target: { value: "18:00" },
      });
      fireEvent.click(
        screen.getByRole("button", { name: "Создать задачу" }),
      );
      expect(screen.getByLabelText("Срок задачи: дата")).toHaveAttribute(
        "aria-invalid",
        "true",
      );
    }
    expect(api).not.toHaveBeenCalled();
  });
  it("sends arbitrary intervals and offers Premium instead of a third row", async () => {
    vi.mocked(api).mockClear();
    vi.mocked(api).mockResolvedValueOnce({});
    render(
      <MemoryRouter>
        <TaskEditor open task={null} close={vi.fn()} />
      </MemoryRouter>,
    );
    fireEvent.change(screen.getByLabelText("Название"), {
      target: { value: "Задача" },
    });
    fireEvent.change(screen.getByLabelText("Срок задачи: дата"), {
      target: { value: "18.10.2099" },
    });
    fireEvent.change(screen.getByLabelText("Срок задачи: время"), {
      target: { value: "18:00" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Добавить напоминание" }),
    );
    fireEvent.change(screen.getByLabelText("Дни: напоминание 1"), {
      target: { value: "1" },
    });
    fireEvent.change(screen.getByLabelText("Часы: напоминание 1"), {
      target: { value: "3" },
    });
    fireEvent.change(screen.getByLabelText("Минуты: напоминание 1"), {
      target: { value: "20" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Добавить напоминание" }),
    );
    expect(
      screen.queryByRole("button", { name: "Добавить напоминание" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Больше — с Premium" }),
    ).toBeVisible();
    fireEvent.click(
      screen.getByRole("button", { name: "Создать задачу" }),
    );
    await waitFor(() => expect(api).toHaveBeenCalled());
    expect(
      JSON.parse(vi.mocked(api).mock.calls[0][1]!.body as string).reminders,
    ).toEqual([{ offset_minutes: 1640 }, { offset_minutes: 0 }]);
  });
  it("maps backend errors to the row without losing input", async () => {
    vi.mocked(api).mockClear();
    vi.mocked(api).mockRejectedValueOnce(
      new ApiError(400, "Время прошло", {
        field_errors: { "reminders.0": "Это напоминание уже в прошлом" },
      }),
    );
    render(
      <MemoryRouter>
        <TaskEditor open task={null} close={vi.fn()} />
      </MemoryRouter>,
    );
    fireEvent.change(screen.getByLabelText("Название"), {
      target: { value: "Черновик" },
    });
    fireEvent.change(screen.getByLabelText("Срок задачи: дата"), {
      target: { value: "18.10.2099" },
    });
    fireEvent.change(screen.getByLabelText("Срок задачи: время"), {
      target: { value: "18:00" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Добавить напоминание" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Создать задачу" }),
    );
    await waitFor(() =>
      expect(screen.getByText("Это напоминание уже в прошлом")).toBeVisible(),
    );
    expect(screen.getByLabelText("Название")).toHaveValue("Черновик");
    expect(screen.getByLabelText("Минуты: напоминание 1")).toHaveValue("0");
  });
  it("keeps the form and draft after a failed save", async () => {
    vi.mocked(api).mockRejectedValueOnce(new Error("offline"));
    const close = vi.fn();
    render(
      <MemoryRouter>
        <TaskEditor open task={null} close={close} />
      </MemoryRouter>,
    );
    fireEvent.change(screen.getByLabelText("Название"), {
      target: { value: "Написать книгу" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Создать задачу/ }));
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("Ошибка сервера"),
    );
    expect(screen.getByLabelText("Название")).toHaveValue("Написать книгу");
    expect(close).not.toHaveBeenCalled();
  });
  it("allows clearing a deadline with explicit null", async () => {
    vi.mocked(api).mockResolvedValueOnce({});
    render(
      <MemoryRouter>
        <TaskEditor
          open
          task={{
            id: 1,
            title: "Задача",
            description: "",
            status: "pending",
            deadline: "2026-10-14T12:00:00Z",
          }}
          close={vi.fn()}
        />
      </MemoryRouter>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Убрать срок" }));
    fireEvent.click(
      screen.getByRole("button", { name: /Сохранить изменения/ }),
    );
    await waitFor(() =>
      expect(api).toHaveBeenLastCalledWith(
        "/tasks/1",
        expect.objectContaining({
          method: "PATCH",
          body: expect.stringContaining('"deadline":null'),
        }),
      ),
    );
  });
});
