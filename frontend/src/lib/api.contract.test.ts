import { describe, expect, it, vi } from "vitest";
import baseline from "../../../backend/internal/http-server/router/testdata/error_contract_baseline.json";
import postgres from "../../../backend/internal/http-server/router/testdata/error_contract_postgres_baseline.json";
import { api, ApiError, errorText, saveTokens, type ApiErrorData } from "./api";
import { command } from "./commands";

interface Fixture {
  status: number;
  json?: ApiErrorData;
}
const fixtures: Record<string, Fixture> = { ...baseline, ...postgres };

describe("HTTP error compatibility", () => {
  // These are the independently reviewed backend fixtures, not copies generated
  // from the frontend implementation. Webhook text responses are not UI APIs.
  it.each(Object.entries(fixtures).filter(([, f]) => f.status >= 400 && f.json))(
    "preserves the backend contract for %s",
    async (_name, fixture) => {
      vi.stubGlobal("fetch", vi.fn(async () => Response.json(fixture.json, { status: fixture.status })));
      await expect(api("/contract", {}, false)).rejects.toMatchObject({
        status: fixture.status,
        message: fixture.json?.message,
        data: fixture.json,
      });
    },
  );

  it.each([
    { body: { error: "legacy response" }, status: 400, message: "legacy response" },
    { body: { status: 404, message: "numeric legacy" }, status: 409, message: "numeric legacy" },
    { body: { status: "ERROR", error: "legacy uppercase" }, status: 200, message: "legacy uppercase" },
    { body: { error: "legacy fields", validation_errors: { title: "required" } }, status: 400, message: "legacy fields" },
  ])("accepts older backend payloads: $message", async ({ body, status, message }) => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json(body, { status })));
    await expect(api("/legacy", {}, false)).rejects.toMatchObject({ status, message, data: body });
  });

  it("uses stable codes with changed text and falls back for unknown codes", () => {
    for (const code of ["CATEGORY_LIMIT_REACHED", "BREAK_ALREADY_USED"]) {
      expect(errorText(new ApiError(409, "changed text", { code }))).toBe(
        errorText(new ApiError(409, "Другая формулировка", { code })),
      );
    }
    expect(errorText(new ApiError(429, "arbitrary text", { code: "FUTURE_CODE" }))).toContain("Слишком много");
    expect(errorText(new ApiError(500, "SQL private cause", { code: "FUTURE_CODE" }))).not.toContain("SQL");
  });
});

describe("command retries across backend versions", () => {
  for (const current of [false, true]) {
    it.each([400, 429, 503])("keeps HTTP semantics with current=" + current + " (%i)", async (status) => {
      let sequence = 0;
      vi.stubGlobal("crypto", { randomUUID: () => `test-key-${++sequence}` });
      saveTokens({ access_token: "contract-test-access" });
      const keys: string[] = [];
      const body = current
        ? { code: status === 429 ? "RATE_LIMITED" : status === 400 ? "VALIDATION_ERROR" : "INTERNAL_ERROR", message: "public error", error: "public error", request_id: "command-request", field_errors: {} }
        : { error: "legacy error" };
      vi.stubGlobal("fetch", vi.fn(async (_path: string, init?: RequestInit) => {
        keys.push((init?.headers as Record<string, string>)["Idempotency-Key"]);
        return keys.length === 1 ? Response.json(body, { status }) : Response.json({ status: "OK" });
      }));
      await expect(command(7, "/tasks", { title: "Task" })).rejects.toBeInstanceOf(ApiError);
      expect(sessionStorage.length).toBe(status === 400 ? 0 : 1);
      await command(7, "/tasks", { title: "Task" });
      if (status === 400) expect(keys[1]).not.toBe(keys[0]);
      else expect(keys[1]).toBe(keys[0]);
      expect(sessionStorage.length).toBe(0);
    });
  }
});
