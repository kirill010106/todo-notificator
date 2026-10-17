import { describe, expect, it, vi } from "vitest";
import { api, ApiError } from "../../lib/api";
import { loadActiveSession } from "./Focus";
vi.mock("../../lib/api", async (original) => ({
  ...(await original<typeof import("../../lib/api")>()),
  api: vi.fn(),
}));
describe("active Pomodoro contract", () => {
  it("treats 404 as idle, including after a task invalidates the queries", async () => {
    vi.mocked(api).mockRejectedValueOnce(
      new ApiError(404, "session not found"),
    );
    expect(await loadActiveSession()).toEqual({ session: null });
  });
  it("still surfaces server failures", async () => {
    vi.mocked(api).mockRejectedValueOnce(
      new ApiError(500, "internal server error"),
    );
    await expect(loadActiveSession()).rejects.toMatchObject({ status: 500 });
  });
});
