import { describe, expect, it, vi } from "vitest";
import { claimDailyReward } from "./xp-rewards";
import { api, ApiError } from "./api";
vi.mock("./api", async (original) => ({
  ...(await original<typeof import("./api")>()),
  api: vi.fn(),
}));
describe("server rewards and idempotent commands", () => {
  it("claims a server-defined daily reward without sending points or a price", async () => {
    vi.mocked(api).mockResolvedValueOnce({ rewards: { quest_xp: 50 } });
    await claimDailyReward(1);
    expect(api).toHaveBeenLastCalledWith("/me/rewards/daily-chest/claim", {
      method: "POST",
      headers: { "Idempotency-Key": expect.any(String) },
    });
  });
  it("keeps the same key after a lost response", async () => {
    vi.mocked(api)
      .mockReset()
      .mockRejectedValueOnce(new TypeError("lost response"))
      .mockResolvedValueOnce({});
    await expect(claimDailyReward(1, "create_task")).rejects.toThrow();
    const first = vi.mocked(api).mock.calls[0][1]?.headers;
    await claimDailyReward(1, "create_task");
    expect(vi.mocked(api).mock.calls[1][1]?.headers).toEqual(first);
  });
  it("accepts an authoritative already-claimed error without changing local coins", async () => {
    localStorage.setItem("focus_coins", "100");
    vi.mocked(api).mockRejectedValueOnce(
      new ApiError(409, "Награда уже получена"),
    );
    await expect(claimDailyReward(1)).rejects.toMatchObject({ status: 409 });
    expect(localStorage.getItem("focus_coins")).toBe("100");
  });
});
