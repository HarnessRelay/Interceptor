import { defineConfig, devices } from "@playwright/test";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

// Hermetic XDG dirs keep the QA daemon away from the developer's real
// HarnessRelay config (paired devices, network lists) and session archive.
const qaHome = mkdtempSync(path.join(tmpdir(), "harnessrelay-qa-"));

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
    timeout: 20_000,
    env: {
      ...process.env,
      XDG_CONFIG_HOME: path.join(qaHome, "config"),
      XDG_DATA_HOME: path.join(qaHome, "data"),
      XDG_STATE_HOME: path.join(qaHome, "state")
    } as Record<string, string>
  },
  projects: [
    {
      name: "chrome",
      use: { ...devices["Desktop Chrome"] }
    }
  ]
});
