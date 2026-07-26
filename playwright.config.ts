import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./qa/playwright",
  timeout: 45_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  reporter: [["list"]],
  outputDir: "qa/artifacts/playwright",
  use: {
    baseURL: "http://127.0.0.1:8767",
    browserName: "chromium",
    launchOptions: {
      executablePath: "/usr/bin/google-chrome"
    },
    headless: true,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    viewport: { width: 1440, height: 960 }
  },
  webServer: {
    command: "HARNESSRELAY_TOKEN=dashboard-token HARNESSRELAY_PORT=8767 HARNESSRELAY_ENABLE_FAKE_ADAPTER=1 ./bin/harnessd serve",
    url: "http://127.0.0.1:8767/api/v1/health",
    reuseExistingServer: false,
    timeout: 20_000
  },
  projects: [
    {
      name: "chrome",
      use: { ...devices["Desktop Chrome"] }
    }
  ]
});
