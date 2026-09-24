import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": "http://127.0.0.1:8090",
      "/healthz": "http://127.0.0.1:8090",
      "/ws": { target: "ws://127.0.0.1:8090", ws: true },
    },
  },
  build: { chunkSizeWarningLimit: 1600 },
});
