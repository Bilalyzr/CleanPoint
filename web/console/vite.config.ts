import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The console is served by the Go API under /app/ (same origin, no CORS).
export default defineConfig({
  base: "/app/",
  plugins: [react()],
  build: { outDir: "../console-dist", emptyOutDir: true, sourcemap: false },
});
