import { describe, expect, it } from "vitest";
import { safeTimeZone } from "./timezone";

describe("safeTimeZone", () => {
  it("falls back for the Go-only Local zone and other stale values", () => {
    expect(safeTimeZone("Local")).toBe("UTC");
    expect(safeTimeZone("not/a-zone")).toBe("UTC");
  });
  it("preserves valid IANA locations", () => {
    expect(safeTimeZone("Europe/Moscow")).toBe("Europe/Moscow");
  });
});
