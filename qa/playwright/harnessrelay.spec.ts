import { expect, test } from "@playwright/test";
import { mkdirSync, writeFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { createSession, expectNoChatGarbage, login, screenshotDir, selectSession, sendChat, sendRaw, snapshotText, waitForSnapshotText, consoleErrors } from "./helpers";

const repoRoot = process.cwd().endsWith(`${path.sep}web`) ? path.dirname(process.cwd()) : process.cwd();

test.beforeAll(() => {
  mkdirSync(screenshotDir, { recursive: true });
});

function unexpectedErrors(errors: string[]) {
  return errors.filter((message) => !/401 \(Unauthorized\)|400 \(Bad Request\)/.test(message));
}

test("Screen 1: Login/Auth", async ({ page, request }) => {
  const errors = await consoleErrors(page);
  const unauthenticated = await request.get("/api/v1/sessions");
  expect(unauthenticated.status()).toBe(401);

  await page.goto("/");
  await expect(page.locator(".login-panel")).toBeVisible();
  await expect(page.locator(".logo-wordmark")).toBeVisible();
  await page.screenshot({ path: `${screenshotDir}/login.png`, fullPage: true });

  const password = page.locator(".login-panel input[type=password]");
  await password.fill("wrong-token");
  await page.locator(".login-panel").evaluate((form) => (form as HTMLFormElement).requestSubmit());
  await expect(page.locator(".login-error")).toContainText(/invalid|unauthorized|token/i);

  await password.fill("dashboard-token");
  await password.press("Enter");
  await expect(page.locator(".session-launcher")).toBeVisible();
  await page.reload();
  await expect(page.locator(".session-launcher")).toBeVisible();
  await expect(unexpectedErrors(errors)).toEqual([]);
});

test("Screen 2: Empty App Shell", async ({ page }) => {
  const errors = await consoleErrors(page);
  await login(page);

  await expect(page.locator(".brand")).toContainText("HarnessRelay");
  await expect(page.locator(".session-launcher")).toBeVisible();
  await expect(page.locator(".session-empty")).toContainText("No sessions yet");
  await expect(page.locator(".empty-state")).toContainText(/detected harness|Checking the local daemon/);
  await expect(page.getByRole("button", { name: "Refresh sessions" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  await page.screenshot({ path: `${screenshotDir}/empty-app-shell.png`, fullPage: true });
  await expect(unexpectedErrors(errors)).toEqual([]);
});

test("Screen 3: Create Session", async ({ page }) => {
  const errors = await consoleErrors(page);
  await login(page);

  await page.getByRole("button", { name: "Manual" }).click();
  const form = page.locator(".create-form");
  await expect(form.locator("label", { hasText: "Name" }).locator("input")).toBeVisible();
  await expect(form.locator("label", { hasText: "Command" }).locator("input")).toHaveValue("/bin/bash");
  await expect(form.locator("label", { hasText: "Args" }).locator("input")).toHaveAttribute("placeholder", "optional arguments");
  await expect(form.locator("label", { hasText: "Args" }).locator("input")).toHaveValue("");
  await expect(form.locator("label", { hasText: "CWD" }).locator("input")).toHaveAttribute("placeholder", "daemon default");
  await expect(form.getByRole("button", { name: "Chat" })).toHaveAttribute("aria-pressed", "true");

  await form.locator("label", { hasText: "Command" }).locator("input").fill("");
  await form.getByRole("button", { name: "Create session" }).click();
  await expect(page.locator(".notice")).toContainText("Command is required.");

  await form.locator("label", { hasText: "Command" }).locator("input").fill("/bin/harnessrelay-missing-command");
  await form.getByRole("button", { name: "Create session" }).click();
  await expect(page.locator(".notice")).toContainText(/no such file|not found|executable/i);

  const unique = Date.now();
  await createSession(page, { name: `pw-create-${unique}`, command: "/bin/echo", args: `create-ok-${unique}`, cwd: "/tmp", mode: "terminal" });
  await expect(page.locator(".session-list .session-item").first()).toContainText(`pw-create-${unique}`);
  await expect(page.locator(".terminal-section")).toBeVisible();
  await page.screenshot({ path: `${screenshotDir}/create-session.png`, fullPage: true });
  await expect(unexpectedErrors(errors)).toEqual([]);
});

test("Screen 4: Chat Mode with simple shell", async ({ page }) => {
  const errors = await consoleErrors(page);
  await login(page);

  const unique = Date.now();
  await createSession(page, { name: `pw-shell-${unique}`, command: "/bin/bash", cwd: "/tmp", mode: "chat" });
  await expect(page.locator(".chat-status-row")).toContainText(/Ready|Streaming text/);

  await sendChat(page, `echo chat-mode-works-${unique}`);
  await expect(page.locator(".transcript")).toContainText(`chat-mode-works-${unique}`);
  await expect(page.locator(".transcript")).toContainText(`echo chat-mode-works-${unique}`);
  await expect(page.locator(".composer textarea")).toBeEditable();

  await page.locator(".composer textarea").fill(`echo enter-sends-${unique}`);
  await page.locator(".composer textarea").press("Enter");
  await expect(page.locator(".transcript")).toContainText(`enter-sends-${unique}`);

  await page.locator(".composer textarea").fill("printf '");
  await page.locator(".composer textarea").press("Shift+Enter");
  await expect(page.locator(".composer textarea")).toHaveValue("printf '\n");

  const longText = `long-output-${unique}-` + "x".repeat(180);
  await sendChat(page, `printf '${longText}\\n'`);
  await expect(page.locator(".transcript")).toContainText(longText);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

  await expectNoChatGarbage(page.locator(".transcript"));
  await page.screenshot({ path: `${screenshotDir}/chat-shell.png`, fullPage: true });

  await page.getByRole("button", { name: "Open Terminal" }).click();
  await expect(page.locator(".terminal-section .raw-input textarea")).toBeVisible();
  await expect(page.locator(".xterm-rows")).toBeVisible();
  await sendRaw(page, `echo terminal-mode-works-${unique}\n`);
  await expect(page.locator("body")).toContainText(`terminal-mode-works-${unique}`);
  await page.screenshot({ path: `${screenshotDir}/terminal-mode.png`, fullPage: true });

  await page.getByRole("button", { name: "Open Chat" }).click();
  await expect(page.locator(".chat-view")).toBeVisible();
  await expect(page.locator(".transcript .message-user", { hasText: `echo chat-mode-works-${unique}` })).toBeVisible();
  await expect(page.locator(".transcript .message-assistant", { hasText: `chat-mode-works-${unique}` }).first()).toBeVisible();

  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "Terminate", exact: true }).click();
  await expect(errors).toEqual([]);
});

test("Screen 6: Slash Command Menu", async ({ page }) => {
  const errors = await consoleErrors(page);
  await login(page);

  const unique = Date.now();
  await createSession(page, { name: `pw-slash-${unique}`, command: "/bin/bash", cwd: "/tmp", mode: "chat" });
  await page.locator(".slash-button").click();
  const menu = page.locator(".slash-menu");
  await expect(menu).toBeVisible();
  await expect(menu).toHaveAttribute("role", "menu");
  await expect(menu.getByRole("menuitem", { name: "Send Enter" })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: "Open Terminal Mode" })).toBeVisible();
  await page.screenshot({ path: `${screenshotDir}/slash-menu.png`, fullPage: true });

  await menu.getByRole("menuitem", { name: "Refresh snapshot" }).click();
  await expect(menu).toBeHidden();

  for (const action of ["Send Escape", "Send Ctrl+C", "Send Enter"]) {
    await page.locator(".slash-button").click();
    await page.locator(".slash-menu").getByRole("menuitem", { name: action }).click();
    await expect(page.locator(".slash-menu")).toBeHidden();
  }

  await page.locator(".slash-button").click();
  await page.locator(".slash-menu").getByRole("menuitem", { name: "Open Terminal Mode" }).click();
  await expect(page.locator(".terminal-section")).toBeVisible();
  await page.getByRole("button", { name: "Open Chat" }).click();
  await expect(page.locator(".chat-view")).toBeVisible();

  await page.locator(".slash-button").click();
  page.once("dialog", async (dialog) => {
    expect(dialog.type()).toBe("confirm");
    await dialog.dismiss();
  });
  await page.locator(".slash-menu").getByRole("menuitem", { name: "Terminate" }).click();
  await expect(page.locator(".chat-view")).toBeVisible();

  await page.locator(".slash-button").click();
  page.once("dialog", async (dialog) => {
    expect(dialog.type()).toBe("prompt");
    await dialog.dismiss();
  });
  await page.locator(".slash-menu").getByRole("menuitem", { name: "Force kill" }).click();
  await expect(page.locator(".chat-view")).toBeVisible();
  await expect(errors).toEqual([]);
});

test("QA-002: Chat Mode suppresses live noisy TUI artifacts consistently", async ({ page }) => {
  const errors = await consoleErrors(page);
  await login(page);

  await page.evaluate(() => {
    (window as Window & { __qaSawNoisyChat?: boolean }).__qaSawNoisyChat = false;
    const observer = new MutationObserver(() => {
      const transcript = document.querySelector(".transcript");
      if (transcript?.textContent?.includes("MMMMMMMM")) {
        (window as Window & { __qaSawNoisyChat?: boolean }).__qaSawNoisyChat = true;
      }
    });
    observer.observe(document.body, { childList: true, subtree: true, characterData: true });
  });

  const unique = Date.now();
  const sessionName = `pw-noisy-${unique}`;
  const scriptPath = path.join(repoRoot, "testdata/fake-harnesses/noisy-tui-artifact.sh");
  await createSession(page, { name: sessionName, command: "/bin/sh", args: scriptPath, cwd: "/tmp", mode: "chat" });

  const transcript = page.locator(".transcript");
  await expect(transcript).toContainText("This session is using a terminal UI");
  await expect(transcript).not.toContainText("MMMMMMMM");
  await expectNoChatGarbage(transcript);
  expect(await page.evaluate(() => (window as Window & { __qaSawNoisyChat?: boolean }).__qaSawNoisyChat)).toBe(false);

  await page.getByRole("button", { name: "Open Terminal" }).click();
  await waitForSnapshotText(page, sessionName, "MMMMMMMM");
  await page.getByRole("button", { name: "Open Chat" }).click();
  await expect(page.locator(".transcript")).toContainText("This session is using a terminal UI");
  await expect(page.locator(".transcript")).not.toContainText("MMMMMMMM");

  await page.reload();
  await expect(page.locator(".session-launcher")).toBeVisible();
  await selectSession(page, sessionName);
  await expect(page.locator(".transcript")).toContainText("This session is using a terminal UI");
  await expect(page.locator(".transcript")).not.toContainText("MMMMMMMM");
  await expect(errors).toEqual([]);
});

test("QA-003: Chat Mode Send and Enter submit a real terminal Enter", async ({ page }) => {
  const errors = await consoleErrors(page);
  await login(page);

  const unique = Date.now();
  const sessionName = `pw-chat-enter-${unique}`;
  const scriptPath = path.join(repoRoot, "testdata/fake-harnesses/ready-received.sh");
  await createSession(page, { name: sessionName, command: "/bin/sh", args: scriptPath, cwd: "/tmp", mode: "chat" });
  await expect(page.locator(".transcript")).toContainText("READY");

  await sendChat(page, `hello-from-chat-${unique}`);
  await expect(page.locator(".transcript")).toContainText(`hello-from-chat-${unique}`);
  await expect(page.locator(".transcript")).toContainText(`RECEIVED:hello-from-chat-${unique}`);

  await page.locator(".composer textarea").fill(`hello-from-enter-${unique}`);
  await page.locator(".composer textarea").press("Enter");
  await expect(page.locator(".transcript")).toContainText(`hello-from-enter-${unique}`);
  await expect(page.locator(".transcript")).toContainText(`RECEIVED:hello-from-enter-${unique}`);
  await expect(errors).toEqual([]);
});

test("Screen 7: Terminal Mode", async ({ page }) => {
  const errors = await consoleErrors(page);
  await login(page);

  const unique = Date.now();
  const sessionName = `pw-terminal-${unique}`;
  await createSession(page, { name: sessionName, command: "/bin/bash", cwd: "/tmp", mode: "terminal" });
  await expect(page.locator(".xterm-rows")).toBeVisible();

  await page.locator(".terminal-host").click();
  await page.keyboard.type(`echo xterm-typed-${unique}`);
  await page.keyboard.press("Enter");
  await waitForSnapshotText(page, sessionName, `xterm-typed-${unique}`);

  await page.locator(".terminal-host").click();
  await page.keyboard.insertText(`echo pasted-${unique}`);
  await page.keyboard.press("Enter");
  await waitForSnapshotText(page, sessionName, `pasted-${unique}`);

  await sendRaw(page, `echo raw-fallback-${unique}\n`);
  await waitForSnapshotText(page, sessionName, `raw-fallback-${unique}`);

  await page.setViewportSize({ width: 1600, height: 1000 });
  await expect.poll(async () => page.evaluate(async (name) => {
    const list = await fetch("/api/v1/sessions", { credentials: "same-origin" }).then((response) => response.json());
    const session = list.sessions.find((item: { name?: string }) => item.name === name);
    return session?.terminal?.cols || 0;
  }, sessionName)).toBeGreaterThan(80);

  await sendRaw(page, `sleep 5\n`);
  await page.getByRole("button", { name: "Interrupt" }).click();
  await sendRaw(page, `echo after-interrupt-${unique}\n`);
  await waitForSnapshotText(page, sessionName, `after-interrupt-${unique}`);

  const eventPanelHeight = await page.locator(".event-panel").boundingBox().then((box) => box?.height || 0);
  expect(eventPanelHeight).toBeLessThan(80);

  await page.getByRole("button", { name: "Open Chat" }).click();
  await expect(page.locator(".chat-view")).toBeVisible();
  await page.getByRole("button", { name: "Open Terminal" }).click();
  await waitForSnapshotText(page, sessionName, `xterm-typed-${unique}`);

  page.once("dialog", async (dialog) => {
    expect(dialog.type()).toBe("prompt");
    await dialog.dismiss();
  });
  await page.getByRole("button", { name: "Force kill" }).click();

  await page.screenshot({ path: `${screenshotDir}/terminal-mode.png`, fullPage: true });
  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "Terminate", exact: true }).click();
  await expect(page.locator(".session-header")).toContainText(/exited|terminated/);
  await expect(errors).toEqual([]);
});

test("Screen 8: Reconnect and Reload", async ({ page }) => {
  const errors = await consoleErrors(page);
  await login(page);

  const unique = Date.now();
  const sessionName = `pw-reconnect-${unique}`;
  await createSession(page, { name: sessionName, command: "/bin/bash", cwd: "/tmp", mode: "chat" });
  await sendChat(page, `echo before-reload-${unique}`);
  await waitForSnapshotText(page, sessionName, `before-reload-${unique}`);

  await page.reload();
  await expect(page.locator(".session-launcher")).toBeVisible();
  await expect(page.getByRole("button", { name: new RegExp(sessionName) })).toBeVisible();
  await selectSession(page, sessionName);
  await expect(page.locator(".transcript")).toContainText(`before-reload-${unique}`);
  await sendChat(page, `echo after-reload-${unique}`);
  await waitForSnapshotText(page, sessionName, `after-reload-${unique}`);
  await expect(page.locator(".transcript")).toContainText(`after-reload-${unique}`);

  await page.getByRole("button", { name: "Open Terminal" }).click();
  await expect(page.locator(".xterm-rows")).toBeVisible();
  await page.getByRole("button", { name: "Open Chat" }).click();
  await expect(page.locator(".chat-view")).toBeVisible();

  const completedName = `pw-completed-${unique}`;
  await createSession(page, { name: completedName, command: "/bin/echo", args: `completed-${unique}`, cwd: "/tmp", mode: "terminal" });
  await waitForSnapshotText(page, completedName, `completed-${unique}`);
  await expect(page.locator(".session-header")).toContainText(/exited|terminated/);
  await page.setViewportSize({ width: 1360, height: 860 });
  await page.waitForTimeout(250);

  await selectSession(page, sessionName);
  await page.screenshot({ path: `${screenshotDir}/reconnect.png`, fullPage: true });
  await expect(errors).toEqual([]);
});

test("Screen 9: Multiple Sessions", async ({ page }) => {
  const errors = await consoleErrors(page);
  await login(page);

  const unique = Date.now();
  const first = `pw-multi-a-${unique}`;
  const second = `pw-multi-b-${unique}`;
  await createSession(page, { name: first, command: "/bin/bash", cwd: "/tmp", mode: "terminal" });
  await sendRaw(page, `echo first-only-${unique}\n`);
  await waitForSnapshotText(page, first, `first-only-${unique}`);

  await createSession(page, { name: second, command: "/bin/bash", cwd: "/tmp", mode: "terminal" });
  await sendRaw(page, `echo second-only-${unique}\n`);
  await waitForSnapshotText(page, second, `second-only-${unique}`);

  await expect(page.locator(".session-list .session-item").first()).toContainText(second);
  await selectSession(page, first);
  await sendRaw(page, `echo first-selected-${unique}\n`);
  await waitForSnapshotText(page, first, `first-selected-${unique}`);
  expect(await snapshotText(page, second)).not.toContain(`first-selected-${unique}`);

  await selectSession(page, second);
  await sendRaw(page, `echo second-selected-${unique}\n`);
  await waitForSnapshotText(page, second, `second-selected-${unique}`);
  expect(await snapshotText(page, first)).not.toContain(`second-selected-${unique}`);

  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "Terminate", exact: true }).click();
  await expect(page.locator(".session-header")).toContainText(/exited|terminated/);
  await selectSession(page, first);
  await sendRaw(page, `echo first-still-running-${unique}\n`);
  await waitForSnapshotText(page, first, `first-still-running-${unique}`);

  await page.screenshot({ path: `${screenshotDir}/multiple-sessions.png`, fullPage: true });
  await expect(errors).toEqual([]);
});

test("QA-001: Chat Mode suppresses full-screen TUI redraw garbage", async ({ page }) => {
  const errors = await consoleErrors(page);
  await login(page);

  const unique = Date.now();
  const tuiBytes = "\\033[?1049h\\033[2J\\033[H\\342\\224\\214\\342\\224\\200 Codex TUI \\342\\224\\200\\342\\224\\220\\r\\n\\342\\224\\202 raw redraw frame \\342\\224\\202\\r\\n\\033[?25l\\033[?1049l";
  await createSession(page, {
    name: `pw-tui-${unique}`,
    command: "/bin/printf",
    args: `"%b" "${tuiBytes}"`,
    mode: "chat"
  });

  const transcript = page.locator(".transcript");
  await expect(transcript).toContainText("This session is using a terminal UI");
  await expect(page.locator(".chat-status-row")).toContainText(/Terminal UI active|Session ended/);
  await expectNoChatGarbage(transcript);
  await expect(page.getByRole("button", { name: "Open Terminal" })).toBeVisible();
  await page.screenshot({ path: `${screenshotDir}/chat-codex.png`, fullPage: true });

  await page.getByRole("button", { name: "Open Terminal" }).click();
  await expect(page.locator(".xterm-rows")).toBeVisible();
  await page.screenshot({ path: `${screenshotDir}/terminal-tui-raw.png`, fullPage: true });
  await expect(errors).toEqual([]);
});

test("Semantic adapter: fake Codex remains coherent across chat, terminal, approval, and reload", async ({ page }) => {
  const errors = await consoleErrors(page);
  await login(page);

  const unique = Date.now();
  const sessionName = `pw-semantic-codex-${unique}`;
  const otherName = `pw-semantic-other-${unique}`;
  const fakeCodex = path.join(repoRoot, "testdata/fake-harnesses/codex");
  await createSession(page, { name: sessionName, command: fakeCodex, cwd: "/tmp", mode: "chat" });

  await expect(page.locator(".adapter-badge")).toHaveText("Codex");
  await expect(page.locator(".session-kicker")).toContainText("Semantic chat");
  await expect.poll(async () => page.evaluate(async (name) => {
    const response = await fetch("/api/v1/sessions", { credentials: "same-origin" });
    const body = await response.json();
    return body.sessions.find((item: { name?: string }) => item.name === name);
  }, sessionName)).toMatchObject({
    adapter_id: "codex",
    adapter_name: "Codex",
    adapter_capabilities: expect.arrayContaining(["semantic_chat", "prompt_submit", "approval_detection"])
  });

  const transcript = page.locator(".transcript");
  await expect(transcript).toContainText("Codex is running in a terminal interface");
  await expect(page.locator(".semantic-strip")).toContainText("gpt-fake high");
  await expect(page.locator(".semantic-strip")).toContainText("Codex 0.145.0");
  await expect(transcript).not.toContainText("MMMMMMMM");
  await expectNoChatGarbage(transcript);
  await expect(page.locator(".chat-status-row")).toContainText("Ready");

  await sendChat(page, `semantic-send-${unique}`);
  await expect(transcript.locator(".message-user", { hasText: `semantic-send-${unique}` })).toBeVisible();
  await waitForSnapshotText(page, sessionName, `RECEIVED:semantic-send-${unique}`);
  await expect(transcript.locator(".message-assistant", { hasText: `Fake Codex response to: semantic-send-${unique}` })).toBeVisible();

  await page.locator(".composer textarea").fill(`semantic-enter-${unique}`);
  await page.locator(".composer textarea").press("Enter");
  await expect(transcript.locator(".message-user", { hasText: `semantic-enter-${unique}` })).toBeVisible();
  await waitForSnapshotText(page, sessionName, `RECEIVED:semantic-enter-${unique}`);
  await expect(transcript.locator(".message-assistant", { hasText: `Fake Codex response to: semantic-enter-${unique}` })).toBeVisible();

  await page.getByRole("button", { name: "Open Terminal" }).click();
  await expect(page.locator(".terminal-section")).toBeVisible();
  await waitForSnapshotText(page, sessionName, "MMMMMMMM");
  await expect(page.locator(".xterm-rows")).toContainText("MMMMMMMM");

  await page.getByRole("button", { name: "Open Chat" }).click();
  await expect(page.locator(".chat-view")).toBeVisible();
  await expect(page.locator(".transcript")).not.toContainText("MMMMMMMM");
  await expect(page.locator(".transcript")).toContainText(`Fake Codex response to: semantic-send-${unique}`);

  await sendChat(page, "request approval");
  const approval = page.locator(".approval-card");
  await expect(approval).toContainText("Approval required");
  await expect(approval).toContainText("$ printf safe");
  await expect(approval.getByRole("button", { name: "Deny" })).toBeVisible();
  await expect(approval.getByRole("button", { name: "Open Terminal" })).toBeVisible();
  await approval.getByRole("button", { name: "Deny" }).click();
  await waitForSnapshotText(page, sessionName, "DENIED");
  await expect(approval).toBeHidden();

  await createSession(page, { name: otherName, command: "/bin/bash", cwd: "/tmp", mode: "chat" });
  await sendChat(page, `other-only-${unique}`);
  await expect(page.locator(".transcript")).toContainText(`other-only-${unique}`);
  await expect(page.locator(".transcript")).not.toContainText(`semantic-send-${unique}`);

  await selectSession(page, sessionName);
  await expect(page.locator(".transcript")).toContainText(`Fake Codex response to: semantic-send-${unique}`);
  await expect(page.locator(".transcript")).not.toContainText(`other-only-${unique}`);
  await expect(page.locator(".transcript")).not.toContainText("MMMMMMMM");

  await page.reload();
  await expect(page.locator(".session-launcher")).toBeVisible();
  await selectSession(page, sessionName);
  await expect(page.locator(".adapter-badge")).toHaveText("Codex");
  await expect(page.locator(".transcript")).toContainText(`Fake Codex response to: semantic-enter-${unique}`);
  await expect(page.locator(".transcript")).not.toContainText("MMMMMMMM");
  await page.screenshot({ path: `${screenshotDir}/semantic-codex.png`, fullPage: true });

  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "Terminate", exact: true }).click();
  await expect(page.locator(".session-header")).toContainText(/exited|terminated/);
  await expect(unexpectedErrors(errors)).toEqual([]);
});

test("QA-001 Codex smoke in disposable directory", async ({ page }) => {
  test.setTimeout(90_000);
  try {
    execFileSync("codex", ["--version"], { stdio: "ignore" });
  } catch {
    test.skip(true, "codex is not installed");
  }

  const errors = await consoleErrors(page);
  const cwd = "/tmp/harnessrelay-qa-codex";
  mkdirSync(cwd, { recursive: true });
  writeFileSync(`${cwd}/README.md`, "# QA sandbox\n\nThis is a disposable test repo.\n");
  try {
    execFileSync("git", ["init"], { cwd, stdio: "ignore" });
    execFileSync("git", ["add", "README.md"], { cwd, stdio: "ignore" });
    execFileSync("git", ["commit", "-m", "init"], { cwd, stdio: "ignore" });
  } catch {
    // Existing disposable repos or missing git identity should not block the UI smoke.
  }

  await login(page);
  const sessionName = `codex-qa-${Date.now()}`;
  await createSession(page, { name: sessionName, command: "codex", cwd, mode: "chat" });
  await expect(page.locator(".adapter-badge")).toHaveText("Codex");
  await expect(page.getByRole("button", { name: "Open Terminal" })).toBeVisible();
  await expect(page.locator(".transcript")).toContainText(/Codex is running in a terminal interface|Waiting for semantic events/);
  await expectNoChatGarbage(page.locator(".transcript"));

  await page.waitForFunction(() => {
    const status = document.querySelector(".chat-status-row")?.textContent || "";
    return document.querySelector(".approval-card") || status.includes("Ready");
  });
  const trustDecision = page.locator(".approval-card", { hasText: "trust this workspace" });
  if (await trustDecision.isVisible()) {
    await trustDecision.getByRole("button", { name: "Open Terminal" }).click();
    await expect(page.locator(".terminal-section")).toBeVisible();
    await page.locator(".terminal-host").click();
    await page.keyboard.press("1");
    await page.keyboard.press("Enter");
    await page.getByRole("button", { name: "Open Chat" }).click();
  }
  await expect(page.locator(".chat-status-row")).toContainText("Ready", { timeout: 30_000 });

  const prompt = "Reply with the three uppercase words semantic, adapter, and ok joined by underscores. Do not use tools or edit files.";
  await sendChat(page, prompt);
  await expect(page.locator(".transcript")).toContainText(prompt);
  await expectNoChatGarbage(page.locator(".transcript"));
  await expect.poll(
    async () => snapshotText(page, sessionName),
    { timeout: 30_000, message: "Codex should submit the Chat prompt and return the requested token" }
  ).toContain("SEMANTIC_ADAPTER_OK");
  await expect(page.locator(".message-assistant")).toContainText("SEMANTIC_ADAPTER_OK", { timeout: 30_000 });
  await expect(page.locator(".semantic-strip")).toContainText(/Model gpt-/, { timeout: 10_000 });
  await page.screenshot({ path: `${screenshotDir}/chat-codex-real.png`, fullPage: true });

  await page.getByRole("button", { name: "Open Terminal" }).click();
  await expect(page.locator(".xterm-rows")).toBeVisible();

  await page.getByRole("button", { name: "Interrupt" }).click();
  await page.waitForTimeout(500);
  const terminate = page.getByRole("button", { name: "Terminate", exact: true });
  if (await terminate.isEnabled()) {
    page.once("dialog", (dialog) => dialog.accept());
    await terminate.click();
  } else {
    await expect(page.locator(".session-header")).toContainText(/exited|terminated/);
  }
  await expect(errors).toEqual([]);
});
