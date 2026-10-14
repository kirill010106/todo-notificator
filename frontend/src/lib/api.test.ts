import { describe, expect, it, vi } from "vitest";
describe("API session handling", () => {
  it("does not let a refresh overwrite credentials from another account", async () => {
    const { refreshAccess, saveTokens } = await import("./api");
    localStorage.setItem("refresh_token", "account-a");
    let resolve!: (response: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((r) => {
            resolve = r;
          }),
      ),
    );
    const pending = refreshAccess();
    saveTokens({
      access_token: "account-b-access",
      refresh_token: "account-b-refresh",
    });
    resolve(Response.json({ access_token: "account-a-access" }));
    await expect(pending).rejects.toThrow();
    expect(localStorage.getItem("access_token")).toBe("account-b-access");
  });
  it("shares one refresh across concurrent 401 responses", async () => {
    const { api } = await import("./api");
    localStorage.setItem("access_token", "old");
    localStorage.setItem("refresh_token", "refresh");
    const fetcher = vi.fn(async (input: string, init?: RequestInit) => {
      if (input.endsWith("/refresh"))
        return Response.json({ access_token: "new", refresh_token: "rotated" });
      return (init?.headers as Record<string, string>).Authorization ===
        "Bearer old"
        ? Response.json({ error: "expired" }, { status: 401 })
        : Response.json({ value: 1 });
    });
    vi.stubGlobal("fetch", fetcher);
    expect(
      await Promise.all([api("/tasks"), api("/categories"), api("/me/stats")]),
    ).toEqual([{ value: 1 }, { value: 1 }, { value: 1 }]);
    expect(
      fetcher.mock.calls.filter(([path]) => path.endsWith("/refresh")),
    ).toHaveLength(1);
  });
  it("clears invalid credentials on rejected refresh", async () => {
    const { api } = await import("./api");
    localStorage.setItem("refresh_token", "bad");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json({ error: "invalid" }, { status: 401 })),
    );
    await expect(api("/tasks")).rejects.toThrow();
    expect(localStorage.getItem("refresh_token")).toBeNull();
  });
  it("does not restore an account after logout during a pending refresh", async () => {
    const { refreshAccess, endSession } = await import("./api");
    localStorage.setItem("refresh_token", "old");
    let resolve!: (response: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((r) => {
            resolve = r;
          }),
      ),
    );
    const pending = refreshAccess();
    endSession();
    resolve(Response.json({ access_token: "stale" }));
    await expect(pending).rejects.toThrow("Сессия завершена");
    expect(localStorage.getItem("access_token")).toBeNull();
  });
  it("keeps credentials on a transient refresh failure", async () => {
    const { refreshAccess } = await import("./api");
    localStorage.setItem("refresh_token", "good");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json({}, { status: 503 })),
    );
    await expect(refreshAccess()).rejects.toThrow();
    expect(localStorage.getItem("refresh_token")).toBe("good");
  });
});
