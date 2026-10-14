import { describe, expect, it } from "vitest";
import { LocalRewardsRepository } from "./rewards";
describe("local rewards migration", () => {
  it("imports legacy data once, preserves originals and isolates accounts", () => {
    localStorage.setItem("focus_coins", "125");
    localStorage.setItem(
      "user_inventory",
      JSON.stringify({
        boosters: { shield: 2 },
        customRewards: [{ id: "r", title: "Книга", price: 20 }],
        activeTheme: "ocean",
      }),
    );
    const repository = new LocalRewardsRepository();
    expect(repository.load(1)).toMatchObject({
      coins: 125,
      boosters: { shield: 2 },
      activeTheme: "ocean",
    });
    expect(repository.load(1).customRewards).toHaveLength(1);
    repository.update(1, (w) => ({ ...w, coins: 100 }));
    expect(repository.load(1).coins).toBe(100);
    expect(repository.load(2).coins).toBe(50);
    expect(localStorage.getItem("focus_coins")).toBe("125");
  });
  it("handles corrupted legacy inventory without losing the wallet", () => {
    localStorage.setItem("user_inventory", "{bad");
    localStorage.setItem("focus_coins", "oops");
    expect(new LocalRewardsRepository().load(1)).toMatchObject({
      coins: 50,
      customRewards: [],
    });
  });
});
