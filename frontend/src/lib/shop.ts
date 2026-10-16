import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { command } from "./commands";
import type { Reward } from "./rewards";
export interface ServerWallet {
  version: number;
  coins: number;
  customRewards: Reward[];
  boosters: Record<string, number>;
  ownedItems: string[];
  activeTheme: string;
  activeSound: string;
  activeBooster: boolean;
  imported: boolean;
}
export interface ShopData {
  wallet: ServerWallet;
  catalog: {
    id: string;
    title: string;
    kind: string;
    price: number;
    description: string;
  }[];
  ledger: {
    id: number;
    coins: number;
    description: string;
    created_at: string;
  }[];
}
export interface RewardsRepository {
  load(userId: number, signal?: AbortSignal): Promise<ShopData>;
  action(
    userId: number,
    body: Record<string, unknown>,
  ): Promise<{ wallet: ServerWallet }>;
}
export class ServerRewardsRepository implements RewardsRepository {
  async load(userId: number, signal?: AbortSignal) {
    return api<ShopData>("/me/shop", { signal });
  }
  action(userId: number, body: Record<string, unknown>) {
    return command<{ wallet: ServerWallet }>(userId, "/me/shop/actions", body);
  }
}
export const shop = new ServerRewardsRepository();
export function importLegacyRewards(userId: number, legacy: { customRewards: Reward[]; activeTheme: string; activeSound: string }) {
  return shop.action(userId, { action: "import_legacy", legacy });
}
export function useShop(userId: number) {
  return useQuery({
    queryKey: ["shop", userId],
    queryFn: ({ signal }) => shop.load(userId, signal),
  });
}
