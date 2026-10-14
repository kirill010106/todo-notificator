import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TaskEditor } from "./Tasks";
import { api } from "../../lib/api";
vi.mock("../../app/context", () => ({
  useApp: () => ({
    data: {
      categories: [],
      user: {
        is_verified: true,
        active_reminders_count: 0,
        reminders_limit: 3,
      },
    },
    refresh: vi.fn(),
    notify: vi.fn(),
  }),
}));
vi.mock("../../lib/api", () => ({
  api: vi.fn(),
  errorText: () => "Ошибка сервера",
  json: JSON.stringify,
}));
describe("Task editor", () => {
  it("keeps the form and draft after a failed save", async () => {
    vi.mocked(api).mockRejectedValueOnce(new Error("offline"));
    const close = vi.fn();
    render(<TaskEditor open task={null} close={close} />);
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
      />,
    );
    fireEvent.change(screen.getByLabelText("Срок"), { target: { value: "" } });
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
