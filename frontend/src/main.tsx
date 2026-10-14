import { createRoot } from "react-dom/client";
import "@fontsource-variable/manrope";
import "./styles/global.css";
import { App } from "./app/App";
import { setTheme } from "./features/account/Account";

try {
  setTheme(localStorage.getItem("todo:theme") || "system");
  document.documentElement.dataset.accent =
    localStorage.getItem("todo:accent") || "default";
} catch {
  /* Default appearance works without persistence. */
}
createRoot(document.getElementById("root")!).render(<App />);
