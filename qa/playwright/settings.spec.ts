import { expect, test } from "@playwright/test";
import { login, token } from "./helpers";

test("Settings: unified view with devices, network, and tunnel tabs", async ({ page }) => {
  await login(page);

  await page.getByRole("button", { name: "Settings" }).click();
  const view = page.locator(".settings-view");
  await expect(view).toBeVisible();
  await expect(view.getByRole("heading", { name: "Settings" })).toBeVisible();

  // Devices tab shows approval guidance and empty states.
  await expect(view.getByRole("heading", { name: "Connection requests" })).toBeVisible();
  await expect(view.getByText(/6-digit code/)).toBeVisible();
  await expect(view.getByRole("heading", { name: "Connected devices" })).toBeVisible();

  // Network tab shows LAN summary, toggle, and lists.
  await view.getByRole("tab", { name: "Network" }).click();
  await expect(view.getByRole("heading", { name: "LAN status" })).toBeVisible();
  const remoteToggle = view.getByRole("switch", { name: /Allow remote devices/ });
  await expect(remoteToggle).toHaveAttribute("aria-checked", "true");

  // Allowlist add/remove round trip.
  await view.locator(".ip-entry-form input").first().fill("192.168.50.0/24");
  await view.getByRole("button", { name: "Add", exact: true }).click();
  await expect(view.locator(".ip-chip")).toContainText("192.168.50.0/24");
  await page.locator(".ip-chip button").first().click();
  await expect(view.locator(".ip-chip")).toHaveCount(0);

  // Remote toggle round trip (host client is never blocked).
  await remoteToggle.click();
  await expect(remoteToggle).toHaveAttribute("aria-checked", "false");
  await remoteToggle.click();
  await expect(remoteToggle).toHaveAttribute("aria-checked", "true");

  // Tunnel tab shows config, binary facts, and the debug console without
  // starting a real tunnel.
  await view.getByRole("tab", { name: "Tunnel" }).click();
  await expect(view.getByRole("heading", { name: "Tunnel status" })).toBeVisible();
  await expect(view.getByRole("heading", { name: "cloudflared binary" })).toBeVisible();
  await expect(view.getByRole("heading", { name: "Debug console" })).toBeVisible();
  await expect(view.getByRole("radio", { name: "Quick Tunnel" })).toBeVisible();
  await expect(view.getByRole("radio", { name: "Named tunnel" })).toBeVisible();

  // Closing settings returns to the sessions workspace.
  await page.getByRole("button", { name: "Close settings and return to sessions" }).click();
  await expect(page.locator(".settings-view")).toHaveCount(0);
});

test("Remote login: device approval flow with verification code", async ({ browser, request }) => {
  // Simulate a remote client: every request carries CF-Connecting-IP, which
  // the daemon (reached over loopback) classifies as a tunnel client.
  const context = await browser.newContext({
    extraHTTPHeaders: { "CF-Connecting-IP": "203.0.113.77" }
  });
  const page = await context.newPage();

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Request device access" })).toBeVisible();

  // The static-token form must not be offered to remote clients.
  await expect(page.locator(".login-panel input[type=password]")).toHaveCount(0);

  // Remote clients cannot log in with the static token even by hand.
  const loginAttempt = await context.request.post("/api/v1/auth/login", {
    data: { token }
  });
  expect(loginAttempt.status()).toBe(403);

  // Request access and read the pairing code.
  await page.getByLabel("This device's name").fill("QA Remote Browser");
  await page.getByRole("button", { name: "Request access" }).click();
  const codeLocator = page.locator(".pairing-code-digits");
  await expect(codeLocator).toBeVisible();
  const code = (await codeLocator.textContent()) || "";
  expect(code).toMatch(/^\d{6}$/);

  // The host dashboard sees the same request with the same code.
  const pending = await request.get("/api/v1/pairing/requests", {
    headers: { Authorization: `Bearer ${token}` }
  });
  expect(pending.status()).toBe(200);
  const body = await pending.json();
  const match = (body.requests || []).find(
    (item: { code?: string; device_name?: string }) => item.code === code && item.device_name === "QA Remote Browser"
  );
  expect(match).toBeTruthy();

  // Approve from the host; the remote browser should sign in automatically.
  const accept = await request.post("/api/v1/pairing/accept", {
    headers: { Authorization: `Bearer ${token}` },
    data: { device_id: match.device_id }
  });
  expect(accept.status()).toBe(200);

  await expect(
    page.getByRole("complementary", { name: "Session manager" })
  ).toBeVisible({ timeout: 15000 });

  // The device token is stored for session re-minting after cookie expiry.
  const stored = await page.evaluate(() => localStorage.getItem("harnessrelay.deviceToken"));
  expect(stored).toMatch(/^hrk_/);

  // The device session can use the API from the remote context.
  const sessions = await context.request.get("/api/v1/sessions");
  expect(sessions.status()).toBe(200);

  await context.close();
});

test("Remote login: rejection is reported and retryable", async ({ browser, request }) => {
  const context = await browser.newContext({
    extraHTTPHeaders: { "CF-Connecting-IP": "203.0.113.78" }
  });
  const page = await context.newPage();

  await page.goto("/");
  await page.getByLabel("This device's name").fill("QA Reject Me");
  await page.getByRole("button", { name: "Request access" }).click();
  await expect(page.locator(".pairing-code-digits")).toBeVisible();

  const pending = await request.get("/api/v1/pairing/requests", {
    headers: { Authorization: `Bearer ${token}` }
  });
  const body = await pending.json();
  const match = (body.requests || []).find(
    (item: { device_name?: string }) => item.device_name === "QA Reject Me"
  );
  expect(match).toBeTruthy();

  await request.post("/api/v1/pairing/reject", {
    headers: { Authorization: `Bearer ${token}` },
    data: { device_id: match.device_id }
  });

  await expect(page.locator(".login-error")).toContainText(/rejected/i, { timeout: 15000 });
  await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();

  await context.close();
});
