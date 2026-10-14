const base = (
  import.meta.env.VITE_API_BASE_URL ||
  (location.hostname === "todo.chkpnk.ru"
    ? "https://todoapi.chkpnk.ru/api/v1"
    : "/api/v1")
).replace(/\/$/, "");
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public data: Record<string, unknown> = {},
  ) {
    super(message);
  }
}
const valid = (value: string | null) =>
  value && !["undefined", "null", ""].includes(value.trim()) ? value : null;
export const hasSession = () =>
  !!(
    valid(localStorage.getItem("access_token")) ||
    valid(localStorage.getItem("refresh_token"))
  );
export function clearTokens() {
  localStorage.removeItem("access_token");
  localStorage.removeItem("refresh_token");
  localStorage.removeItem("user_email");
}
export function saveTokens(data: {
  access_token: string;
  refresh_token?: string;
}) {
  localStorage.setItem("access_token", data.access_token);
  if (data.refresh_token)
    localStorage.setItem("refresh_token", data.refresh_token);
}
let refreshing: Promise<void> | null = null;
// Epoch prevents a refresh from an old account restoring credentials after logout.
let epoch = 0;
export function endSession() {
  epoch++;
  clearTokens();
  window.dispatchEvent(new Event("session-ended"));
}
async function decode(response: Response): Promise<Record<string, unknown>> {
  try {
    return await response.json();
  } catch {
    return {};
  }
}
export async function refreshAccess() {
  if (!refreshing) {
    const currentEpoch = epoch;
    refreshing = (async () => {
      const token = valid(localStorage.getItem("refresh_token"));
      if (!token) {
        endSession();
        throw new ApiError(401, "Войдите в аккаунт заново.");
      }
      const response = await fetch(`${base}/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: token }),
      });
      const data = await decode(response);
      if (
        currentEpoch !== epoch ||
        localStorage.getItem("refresh_token") !== token
      )
        throw new ApiError(401, "Сессия завершена.");
      if (!response.ok || typeof data.access_token !== "string") {
        if (response.status === 401 || response.status === 403) endSession();
        throw new ApiError(
          response.status,
          "Не удалось обновить сессию. Попробуйте ещё раз.",
          data,
        );
      }
      saveTokens(data as { access_token: string; refresh_token?: string });
    })().finally(() => {
      refreshing = null;
    });
  }
  return refreshing;
}
export async function api<T = Record<string, unknown>>(
  path: string,
  init: RequestInit = {},
  authenticated = true,
): Promise<T> {
  const requestEpoch = epoch;
  if (authenticated && !valid(localStorage.getItem("access_token")))
    await refreshAccess();
  const sentToken = localStorage.getItem("access_token");
  const send = () =>
    fetch(`${base}${path}`, {
      ...init,
      headers: {
        ...(init.body ? { "Content-Type": "application/json" } : {}),
        ...init.headers,
        ...(authenticated
          ? { Authorization: `Bearer ${localStorage.getItem("access_token")}` }
          : {}),
      },
    });
  let response = await send();
  if (authenticated && response.status === 401) {
    if (requestEpoch !== epoch) throw new ApiError(401, "Сессия завершена.");
    if (sentToken === localStorage.getItem("access_token"))
      await refreshAccess();
    response = await send();
  }
  if (authenticated && requestEpoch !== epoch)
    throw new ApiError(401, "Сессия завершена.");
  const data = await decode(response);
  if (!response.ok || data.status === "Error" || data.status === "ERROR") {
    if (authenticated && response.status === 401) endSession();
    const message =
      typeof data.message === 'string' ? data.message : typeof data.error === "string"
        ? data.error
        : "Не удалось выполнить запрос. Попробуйте ещё раз.";
    throw new ApiError(response.status, message, data);
  }
  return data as T;
}
export const json = (value: unknown) => JSON.stringify(value);
export const errorText = (error: unknown) =>
  error instanceof ApiError
    ? error.message
    : "Нет связи с сервером. Проверьте подключение и попробуйте ещё раз.";
