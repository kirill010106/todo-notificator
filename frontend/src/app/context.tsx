import {
  createContext,
  useContext,
  useState,
  useCallback,
  type ReactNode,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, hasSession } from "../lib/api";
import type { Bootstrap } from "../lib/types";
import { chime } from "../lib/sound";
import s from "../styles/App.module.css";

const Context = createContext<{
  data: Bootstrap;
  notify: (message: string) => void;
  refresh: () => Promise<void>;
  award: (event: string, coins: number) => void;
} | null>(null);
export const useApp = () => {
  const value = useContext(Context);
  if (!value) throw new Error("Missing app provider");
  return value;
};
export function useBootstrap() {
  return useQuery({
    queryKey: ["bootstrap"],
    enabled: hasSession(),
    queryFn: async ({ signal }) => {
      const value = await api<Bootstrap>("/me/bootstrap", { signal });
      return {
        ...value,
        categories: value.categories || [],
        tasks: value.tasks || [],
      };
    },
    staleTime: 30_000,
  });
}
export function AppProvider({
  data,
  children,
}: {
  data: Bootstrap;
  children: ReactNode;
}) {
  const client = useQueryClient();
  const [message, setMessage] = useState("");
  const notify = useCallback((text: string) => {
    setMessage(text);
  }, []);
  const refresh = useCallback(async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ["bootstrap"] }),
      client.invalidateQueries({ queryKey: ["tasks"] }),
      client.invalidateQueries({ queryKey: ["pending"] }),
      client.invalidateQueries({ queryKey: ["achievements"] }),
      client.invalidateQueries({ queryKey: ["logs"] }),
      client.invalidateQueries({ queryKey: ["session"] }),
      client.invalidateQueries({ queryKey: ["focus"] }),
      client.invalidateQueries({ queryKey: ["shop"] }),
    ]);
  }, [client]);
  const award = (_event: string, _coins: number) => {
    chime();
  };
  return (
    <Context.Provider value={{ data, notify, refresh, award }}>
      {children}
      {message && (
        <div className={s.toast} role="status">
          <span>{message}</span>
          <button
            onClick={() => setMessage("")}
            aria-label="Скрыть уведомление"
          >
            ×
          </button>
        </div>
      )}
    </Context.Provider>
  );
}
