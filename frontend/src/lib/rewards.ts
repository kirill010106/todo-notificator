export interface Reward {
  id: string;
  title: string;
  price: number;
  icon?: string;
}
export interface Wallet {
  version: 1;
  coins: number;
  customRewards: Reward[];
  boosters: Record<string, number>;
  activeTheme: string;
  activeSound: string;
  claimed: string[];
}
export interface RewardsRepository {
  load(userId: number): Wallet;
  update(userId: number, change: (wallet: Wallet) => Wallet): Wallet;
}
const initial = (): Wallet => ({
  version: 1,
  coins: 50,
  customRewards: [],
  boosters: {},
  activeTheme: "default",
  activeSound: "default",
  claimed: [],
});
const parse = (value: string | null): Record<string, unknown> => {
  try {
    const data = JSON.parse(value || "{}");
    return data && typeof data === "object" ? data : {};
  } catch {
    return {};
  }
};
export class LocalRewardsRepository implements RewardsRepository {
  constructor(private storage: Storage = localStorage) {}
  load(userId: number): Wallet {
    const key = `todo:rewards:v1:${userId}`;
    const saved = this.storage.getItem(key);
    if (saved) return { ...initial(), ...parse(saved) } as Wallet;
    const wallet = initial();
    const ownerKey = "todo:rewards:legacy-owner";
    if (!this.storage.getItem(ownerKey)) {
      const inventory = parse(this.storage.getItem("user_inventory"));
      const coins = Number(this.storage.getItem("focus_coins") ?? 50);
      wallet.coins = Number.isFinite(coins) ? Math.max(0, coins) : 50;
      if (Array.isArray(inventory.customRewards))
        wallet.customRewards = inventory.customRewards.filter(
          (r): r is Reward =>
            !!r &&
            typeof r.title === "string" &&
            typeof r.id === "string" &&
            Number.isFinite(r.price) &&
            r.price > 0,
        );
      if (inventory.boosters && typeof inventory.boosters === "object")
        wallet.boosters = inventory.boosters as Record<string, number>;
      if (typeof inventory.activeTheme === "string")
        wallet.activeTheme = inventory.activeTheme;
      if (typeof inventory.activeSound === "string")
        wallet.activeSound = inventory.activeSound;
      this.storage.setItem(key, JSON.stringify(wallet));
      this.storage.setItem(ownerKey, String(userId));
    } else this.storage.setItem(key, JSON.stringify(wallet));
    return wallet;
  }
  update(userId: number, change: (wallet: Wallet) => Wallet): Wallet {
    const next = change(this.load(userId));
    this.storage.setItem(`todo:rewards:v1:${userId}`, JSON.stringify(next));
    return next;
  }
}
