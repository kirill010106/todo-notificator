import { command } from "./commands";
export const claimDailyReward = (userId: number, code?: string) =>
  command(
    userId,
    code
      ? `/me/quests/${encodeURIComponent(code)}/claim`
      : "/me/rewards/daily-chest/claim",
  );
