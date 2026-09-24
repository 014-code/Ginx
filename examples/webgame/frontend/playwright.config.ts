import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "../../../test/webgame",
  outputDir: "./test-results",
  fullyParallel: false,
  workers: 1,
  timeout: 45000,
  use: {
    baseURL: process.env.GINX_WEB_URL || "http://127.0.0.1:8090",
    browserName: "chromium",
    headless: true,
    viewport: { width: 1440, height: 1050 },
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});
