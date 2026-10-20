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
let observedIdentity: string | null | undefined;
function tokenIdentity(token: string | null) {
  if (!token) return null;
  try {
    const part = token.split(".")[1];
    if (!part) return `token:${token}`;
    const payload = JSON.parse(
      atob(part.replace(/-/g, "+").replace(/_/g, "/")),
    );
    return payload &&
      (typeof payload.uid === "string" || typeof payload.uid === "number")
      ? `user:${payload.uid}`
      : `token:${token}`;
  } catch {
    return `token:${token}`;
  }
}
function syncSessionIdentity() {
  const identity = tokenIdentity(valid(localStorage.getItem("access_token")));
  if (observedIdentity === undefined) {
    observedIdentity = identity;
    return false;
  } else if (identity !== observedIdentity) {
    observedIdentity = identity;
    epoch++;
    refreshing = null;
    return true;
  }
  return false;
}
export function sessionStorageChanged() {
  return syncSessionIdentity();
}
export function endSession() {
  epoch++;
  clearTokens();
  observedIdentity = null;
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
  syncSessionIdentity();
  if (!refreshing) {
    const currentEpoch = epoch;
    const requestedToken = valid(localStorage.getItem("refresh_token"));
    const identity = tokenIdentity(valid(localStorage.getItem("access_token")));
    if (!navigator.locks) {
      // An uncoordinated refresh can revoke a valid session in another tab.
      endSession();
      throw new ApiError(
        401,
        "Для обновления сессии нужен современный браузер и HTTPS.",
      );
    }
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 15_000);
    const pending = Promise.resolve(
      navigator.locks.request(
        "todo-notificator:refresh",
        { signal: controller.signal },
        async () => {
          syncSessionIdentity();
          if (currentEpoch !== epoch)
            throw new ApiError(401, "Сессия завершена.");
          const token = valid(localStorage.getItem("refresh_token"));
          if (token !== requestedToken) {
            // A previous lock holder already rotated this account's credentials.
            // Never adopt credentials belonging to a different/unknown account.
            if (
              token &&
              identity &&
              identity ===
                tokenIdentity(valid(localStorage.getItem("access_token")))
            )
              return;
            throw new ApiError(401, "Сессия завершена.");
          }
          if (!token) {
            endSession();
            throw new ApiError(401, "Войдите в аккаунт заново.");
          }
          const response = await fetch(`${base}/refresh`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ refresh_token: token }),
            signal: controller.signal,
          });
          const data = await decode(response);
          syncSessionIdentity();
          if (
            currentEpoch !== epoch ||
            localStorage.getItem("refresh_token") !== token
          )
            throw new ApiError(401, "Сессия завершена.");
          if (!response.ok || typeof data.access_token !== "string") {
            if (response.status === 401 || response.status === 403)
              endSession();
            throw new ApiError(
              response.status,
              "Не удалось обновить сессию. Попробуйте ещё раз.",
              data,
            );
          }
          saveTokens(data as { access_token: string; refresh_token?: string });
          syncSessionIdentity();
        },
      ),
    ).finally(() => {
      clearTimeout(timeout);
      if (refreshing === pending) refreshing = null;
    });
    refreshing = pending;
  }
  return refreshing;
}
export async function api<T = Record<string, unknown>>(
  path: string,
  init: RequestInit = {},
  authenticated = true,
): Promise<T> {
  syncSessionIdentity();
  if (authenticated && !valid(localStorage.getItem("access_token")))
    await refreshAccess();
  const requestEpoch = epoch;
  const sentToken = localStorage.getItem("access_token");
  let lastSentToken = sentToken;
  const send = () => {
    syncSessionIdentity();
    if (authenticated && requestEpoch !== epoch)
      throw new ApiError(401, "Сессия завершена.");
    let body = init.body;
    // Re-read the rotated token for logout retries after a 401.
    if (path === "/logout" && typeof body === "string") {
      try {
        const payload = JSON.parse(body);
        if (
          payload &&
          typeof payload === "object" &&
          "refresh_token" in payload
        )
          body = JSON.stringify({
            ...payload,
            refresh_token: localStorage.getItem("refresh_token"),
          });
      } catch {
        // Preserve malformed input so that the server reports its usual error.
      }
    }
    lastSentToken = localStorage.getItem("access_token");
    return fetch(`${base}${path}`, {
      ...init,
      body,
      headers: {
        ...(init.body ? { "Content-Type": "application/json" } : {}),
        ...init.headers,
        ...(authenticated ? { Authorization: `Bearer ${lastSentToken}` } : {}),
      },
    });
  };
  let response = await send();
  syncSessionIdentity();
  if (authenticated && response.status === 401) {
    if (requestEpoch !== epoch) throw new ApiError(401, "Сессия завершена.");
    if (sentToken === localStorage.getItem("access_token"))
      await refreshAccess();
    response = await send();
  }
  syncSessionIdentity();
  if (authenticated && requestEpoch !== epoch)
    throw new ApiError(401, "Сессия завершена.");
  const data = await decode(response);
  syncSessionIdentity();
  if (authenticated && requestEpoch !== epoch)
    throw new ApiError(401, "Сессия завершена.");
  if (!response.ok || data.status === "Error" || data.status === "ERROR") {
    if (
      authenticated &&
      response.status === 401 &&
      lastSentToken === localStorage.getItem("access_token")
    )
      endSession();
    const message =
      typeof data.message === "string"
        ? data.message
        : typeof data.error === "string"
          ? data.error
          : "Не удалось выполнить запрос. Попробуйте ещё раз.";
    throw new ApiError(response.status, message, data);
  }
  return data as T;
}
export const json = (value: unknown) => JSON.stringify(value);
const messages: Record<string, string> = {
  SESSION_NOT_FOUND:
    "Сессия уже закончена или не найдена. Обнови состояние фокуса.",
  CANCEL_WINDOW_EXPIRED:
    "Прошло больше 30 секунд. Теперь сессию можно завершить обычным способом.",
  BREAK_ALREADY_USED: "Дополнительный перерыв уже использован в этой сессии.",
  CATEGORY_LIMIT_REACHED:
    "В бесплатном тарифе доступно 3 категории. Удали ненужную или подключи Premium.",
  REMINDER_LIMIT_REACHED:
    "В бесплатном тарифе доступно 2 напоминания на задачу. Удали лишнее или подключи Premium.",
  EMAIL_NOT_VERIFIED:
    "Подтверди email в настройках аккаунта, чтобы продолжить.",
  INVALID_CREDENTIALS:
    "Не совпали email или пароль. Проверь их и попробуй снова.",
  INSUFFICIENT_COINS:
    "Не хватает монет. Завершай задачи и засчитанные сессии фокуса, чтобы накопить.",
};
export function errorText(error: unknown): string {
  if (!(error instanceof ApiError))
    return "Нет связи с сервером. Проверь подключение и попробуй ещё раз.";
  const code = String(error.data.code || "");
  if (messages[code]) return messages[code];
  if (error.status === 429)
    return "Слишком много действий подряд. Подожди немного и попробуй ещё раз.";
  if (error.status >= 500)
    return "Сервер временно не смог выполнить действие. Попробуй ещё раз — введённые данные сохранены.";
  if (error.status === 401) return "Войди в аккаунт заново, чтобы продолжить.";
  // Show specific Russian validation messages, but never raw SQL/SMTP or mojibake.
  if (
    /[а-яё]/i.test(error.message) &&
    !/(Р[°µЎџ]|С‚|SQL|smtp|postgres)/i.test(error.message)
  )
    return error.message;
  if (/session not found/i.test(error.message))
    return messages.SESSION_NOT_FOUND;
  if (/invalid credentials|incorrect password/i.test(error.message))
    return messages.INVALID_CREDENTIALS;
  if (/already.*session|active.*session/i.test(error.message))
    return "Уже есть активная сессия. Открой раздел «Фокус», чтобы продолжить.";
  if (error.status === 403)
    return "Это действие недоступно. Проверь подтверждение email и условия своего тарифа в аккаунте.";
  if (error.status === 404)
    return "Объект уже удалён или недоступен. Обнови страницу.";
  if (error.status === 409)
    return "Данные уже изменились. Обнови страницу и попробуй снова.";
  return "Проверь заполненные поля и попробуй ещё раз.";
}
