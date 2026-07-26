import { expect, type Locator, type Page } from "@playwright/test";

export const token = "dashboard-token";
export const screenshotDir = "../qa/artifacts/screenshots";

export async function login(page: Page) {
  await page.goto("/");
  const password = page.locator(".login-panel input[type=password]");
  if (await password.isVisible()) {
    await password.fill(token);
    await page.locator(".login-panel").evaluate((form) => (form as HTMLFormElement).requestSubmit());
  }
  await expect(page.locator(".session-launcher")).toBeVisible();
}

export async function createSession(page: Page, options: { name: string; command: string; args?: string; cwd?: string; mode?: "chat" | "terminal" }) {
  const form = page.locator(".create-form");
  if (!(await form.isVisible())) {
    await page.getByRole("button", { name: "Manual" }).click();
  }
  await expect(form).toBeVisible();
  await form.locator("label", { hasText: "Name" }).locator("input").fill(options.name);
  await form.locator("label", { hasText: "Command" }).locator("input").fill(options.command);
  await form.locator("label", { hasText: "Args" }).locator("input").fill(options.args || "");
  await form.locator("label", { hasText: "CWD" }).locator("input").fill(options.cwd || "");
  await form.getByRole("button", { name: options.mode === "terminal" ? "Terminal" : "Chat" }).click();
  await form.getByRole("button", { name: "Create session" }).click();
  await expect(page.getByRole("button", { name: new RegExp(options.name) })).toBeVisible();
}

export async function sendChat(page: Page, value: string) {
  const composer = page.locator(".composer");
  await composer.locator("textarea").fill(value);
  await composer.getByRole("button", { name: "Send" }).click();
}

export async function sendRaw(page: Page, value: string) {
  const rawInput = page.locator(".raw-input");
  await rawInput.locator("textarea").fill(value);
  await rawInput.getByRole("button", { name: "Send" }).click();
}

export async function selectSession(page: Page, name: string) {
  await page.getByRole("button", { name: new RegExp(name) }).click();
}

export async function snapshotText(page: Page, sessionName: string) {
  return page.evaluate(async (name) => {
    const list = await fetch("/api/v1/sessions", { credentials: "same-origin" }).then((response) => response.json());
    const session = list.sessions.find((item: { name?: string }) => item.name === name);
    if (!session) return "";
    const snapshot = await fetch(`/api/v1/sessions/${session.id}/snapshot`, { credentials: "same-origin" }).then((response) => response.json());
    return snapshot.chunks.map((chunk: { bytes: string }) => atob(chunk.bytes)).join("");
  }, sessionName);
}

export async function waitForSnapshotText(page: Page, sessionName: string, expected: string | RegExp) {
  if (typeof expected === "string") {
    await expect.poll(async () => snapshotText(page, sessionName)).toContain(expected);
    return;
  }
  await expect.poll(async () => snapshotText(page, sessionName)).toMatch(expected);
}

export async function expectNoChatGarbage(transcript: Locator) {
  await expect(transcript).not.toContainText(/[┌┐└┘├┤┬┴┼─│╭╮╰╯═║╔╗╚╝╠╣╦╩╬]/);
  await expect(transcript).not.toContainText(/(?:â|ã|□|�|\x1b|\[[0-?]*[ -/]*[@-~])/);
}

export async function consoleErrors(page: Page) {
  const errors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  page.on("pageerror", (error) => errors.push(error.message));
  return errors;
}
