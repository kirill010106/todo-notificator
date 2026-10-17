import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  // Vite resolves envDir relative to the frontend project root, on Windows too.
  envDir: "..",
  envPrefix: "VITE_",
  plugins: [react()],
  build: {
    rollupOptions: { input: ["index.html", "offer.html", "policy.html"] },
  },
  server: {
    headers: { "Referrer-Policy": "no-referrer" },
    proxy: { "/api": "http://127.0.0.1:8082" },
  },
  test: {
    pool: "threads",
    maxWorkers: 2,
    include: ["src/**/*.test.{ts,tsx}"],
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    restoreMocks: true,
  },
});
