export function safeTimeZone(value?: string | null): string {
  if (value) {
    try {
      return new Intl.DateTimeFormat("ru-RU", { timeZone: value })
        .resolvedOptions().timeZone;
    } catch {
      // Fall back for stale values written by older clients or external tools.
    }
  }
  return "UTC";
}
