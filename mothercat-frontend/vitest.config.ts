import { fileURLToPath } from "node:url";

import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: [fileURLToPath(new URL("./vitest.setup.ts", import.meta.url))],
    restoreMocks: true,
  },
});
