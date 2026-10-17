import { describe, expect, it } from "vitest";
import { secondsLeft } from "./Focus";
import type { Session } from "../../lib/types";

describe("server-based focus clock", () => {
  const started = Date.parse("2026-10-14T12:00:00Z");
  const session: Session = {
    id: 1,
    started_at: new Date(started).toISOString(),
    duration_minutes: 25,
    status: "active",
  };
  it("restores elapsed time instead of restarting a session", () => {
    expect(secondsLeft(session, started + 10 * 60_000)).toBe(15 * 60);
  });
  it("never shows more than 25 minutes while waiting for the next clock tick", () => {
    expect(secondsLeft(session, started - 800)).toBe(1500);
    expect(secondsLeft(session, started + 1)).toBe(1500);
  });
  it("accounts for time spent in a background tab", () => {
    expect(secondsLeft(session, started + 24 * 60_000)).toBe(60);
    expect(secondsLeft(session, started + 26 * 60_000)).toBe(0);
  });
});
