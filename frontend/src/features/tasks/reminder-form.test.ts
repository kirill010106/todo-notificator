import { describe, it, expect } from "vitest";
import {
  newDraft,
  offsetFor,
  parseDate,
  reminderPayload,
  splitDate,
} from "./reminder-form";

describe("deadline and reminder draft validation", () => {
  it.each([
    ["15.10.2026", "", "Дата выбрана"],
    ["", "12:00", "Время указано"],
    ["15.10.20", "12:00", "полностью"],
    ["15.10.2026", "12:", "полностью"],
    ["31.02.2026", "12:00", "не существует"],
    ["15.10.2026", "24:00", "не существует"],
  ])("rejects incomplete and normalized dates %s %s", (date, time, error) =>
    expect(parseDate(date, time).error).toContain(error),
  );
  it("allows empty deadline and round-trips local instants", () => {
    expect(parseDate("", "")).toEqual({});
    const d = parseDate("18.10.2026", "18:00").value!;
    expect(splitDate(d.toISOString())).toEqual({
      date: "18.10.2026",
      time: "18:00",
    });
  });
  it("supports arbitrary interval and equivalent calendar input", () => {
    const deadline = parseDate("18.10.2026", "18:00").value!;
    const row = { ...newDraft(), days: "1", hours: "3", minutes: "20" };
    expect(offsetFor(row, deadline)).toEqual({ offset: 1640 });
    const absolute = new Date(deadline.getTime() - 1640 * 60000);
    expect(
      offsetFor(
        { ...row, mode: "calendar", ...splitDate(absolute.toISOString()) },
        deadline,
      ),
    ).toEqual({ offset: 1640 });
  });
  it("rejects incomplete, negative and fractional intervals", () => {
    for (const minutes of ["", "-1", "1.5", "a", "999999999999999999"])
      expect(offsetFor({ ...newDraft(), minutes }).error).toBeTruthy();
  });
  it("rejects calendar after deadline and missing date", () => {
    const deadline = parseDate("18.10.2026", "18:00").value!;
    expect(
      offsetFor(
        { ...newDraft(), mode: "calendar", date: "18.10.2026", time: "18:01" },
        deadline,
      ).error,
    ).toBeTruthy();
    expect(
      offsetFor({ ...newDraft(), mode: "calendar" }, deadline).error,
    ).toBeTruthy();
  });
  it("checks dates, duplicates and free per-task limit", () => {
    const deadline = new Date(Date.now() + 86400000),
      row = newDraft();
    expect(
      reminderPayload([row], undefined, false).errors.deadline,
    ).toBeTruthy();
    expect(
      reminderPayload([row, row], deadline, true).errors["reminders.1"],
    ).toBeTruthy();
    const rows = [0, 1, 2].map((v) => ({ ...newDraft(), minutes: String(v) }));
    expect(
      reminderPayload(rows, deadline, false).errors.reminders,
    ).toBeTruthy();
    expect(reminderPayload(rows, deadline, true).errors).toEqual({});
    expect(
      reminderPayload([row], new Date(0), false).errors["reminders.0"],
    ).toContain("прошлом");
  });
  it("preserves migrated exceptions and sent times", () => {
    const legacy = newDraft({
      id: 3,
      offset_minutes: null,
      scheduled_at: "2026-01-01T00:00:00Z",
      status: "scheduled",
      sent_at: null,
      legacy_absolute: true,
    });
    const result = reminderPayload([legacy], undefined, false);
    expect(result.errors).toEqual({});
    expect(result.reminders).toEqual([{ id: 3, offset_minutes: null }]);
    const sent = newDraft({
      id: 4,
      offset_minutes: 0,
      scheduled_at: "2026-01-01T01:00:00Z",
      status: "sent",
      sent_at: null,
      legacy_absolute: false,
    });
    expect(
      reminderPayload([sent], new Date(Date.now() + 3600000), false).errors,
    ).toEqual({});
  });
});
