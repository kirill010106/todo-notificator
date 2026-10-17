import { describe, expect, it, vi } from "vitest";
import { allPending, todayGroups, localInput } from "./tasks";
import type { Task } from "./types";
vi.mock("./api", () => ({ api: vi.fn() }));
import { api } from "./api";
const task = (id: number, deadline?: string): Task => ({
  id,
  title: "Task",
  description: "",
  status: "pending",
  deadline,
});
describe("Today", () => {
  it("uses local midnight and excludes completed, undated and future tasks", () => {
    const now = new Date(2026, 9, 14, 18);
    const yesterday = new Date(2026, 9, 13, 23, 59).toISOString();
    const today = new Date(2026, 9, 14).toISOString();
    const tomorrow = new Date(2026, 9, 15).toISOString();
    const groups = todayGroups(
      [
        task(1, yesterday),
        task(2, today),
        task(3, tomorrow),
        task(4),
        { ...task(5, today), status: "done" },
      ],
      now,
    );
    expect(groups.overdue.map((t) => t.id)).toEqual([1]);
    expect(groups.today.map((t) => t.id)).toEqual([2]);
    expect(
      todayGroups([task(2, today)], new Date(2026, 9, 15)).overdue,
    ).toHaveLength(1);
  });
  it("loads every page before resolving", async () => {
    vi.mocked(api)
      .mockResolvedValueOnce({
        tasks: Array.from({ length: 100 }, (_, i) => task(i)),
        pagination: { total: 101 },
      })
      .mockResolvedValueOnce({
        tasks: [task(100)],
        pagination: { total: 101 },
      });
    expect(await allPending()).toHaveLength(101);
    expect(api).toHaveBeenLastCalledWith(
      expect.stringContaining("offset=100"),
      { signal: undefined },
    );
  });
  it("rejects incomplete loads rather than displaying partial totals", async () => {
    vi.mocked(api)
      .mockResolvedValueOnce({ tasks: [task(1)], pagination: { total: 2 } })
      .mockRejectedValueOnce(new Error("Network"));
    await expect(allPending()).rejects.toThrow("Network");
  });
  it("prevents an empty page from causing an infinite pagination loop", async () => {
    vi.mocked(api).mockResolvedValueOnce({
      tasks: [],
      pagination: { total: 2 },
    });
    await expect(allPending()).rejects.toThrow("Incomplete");
  });
  it("round trips local datetime input", () => {
    const value = new Date(2026, 9, 14, 17, 32).toISOString();
    expect(new Date(localInput(value)).toISOString()).toBe(value);
  });
});
