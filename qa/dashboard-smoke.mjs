const baseURL = process.env.HARNESSRELAY_DASHBOARD_URL || "http://127.0.0.1:8767/";
const token = process.env.HARNESSRELAY_TOKEN || "dashboard-token";
const cdpURL = process.env.CHROME_CDP_URL || "http://127.0.0.1:9222/json/list";
const chatSessionName = `dash-chat-${Date.now()}`;
const terminalSessionName = `dash-terminal-${Date.now()}`;
const chatLine = `chat smoke ${Date.now()}`;
const terminalLine = `terminal smoke ${Date.now()}`;
const terminalModeLine = `terminal mode ${Date.now()}`;

const pages = await getJSON(cdpURL);
const page = pages.find((entry) => entry.type === "page");
if (!page?.webSocketDebuggerUrl) {
  throw new Error("no Chrome page target found");
}

const cdp = await connectCDP(page.webSocketDebuggerUrl);
const diagnostics = [];
cdp.on("Runtime.consoleAPICalled", (event) => {
  diagnostics.push({ type: "console", level: event.type, args: event.args?.map((arg) => arg.value || arg.description) });
});
cdp.on("Runtime.exceptionThrown", (event) => {
  diagnostics.push({
    type: "exception",
    text: event.exceptionDetails?.text,
    description: event.exceptionDetails?.exception?.description
  });
});
await cdp.send("Runtime.enable");
await cdp.send("Page.enable");
await cdp.send("Page.navigate", { url: baseURL });
await delay(500);

const result = await cdp.send("Runtime.evaluate", {
  awaitPromise: true,
  returnByValue: true,
  expression: `(${dashboardSmoke})(${JSON.stringify({ token, chatSessionName, terminalSessionName, chatLine, terminalLine, terminalModeLine })})`
});

const value = result.result.value;
if (!value?.ok) {
  if (diagnostics.length > 0) value.diagnostics = diagnostics;
  throw new Error(`dashboard smoke failed: ${JSON.stringify(value, null, 2)}`);
}

await cdp.send("Page.navigate", { url: baseURL });
await delay(500);
const reconnect = await cdp.send("Runtime.evaluate", {
  awaitPromise: true,
  returnByValue: true,
  expression: `(${reconnectSmoke})(${JSON.stringify({ chatSessionName, chatLine, terminalLine })})`
});
if (!reconnect.result.value?.ok) {
  throw new Error(`dashboard reconnect failed: ${JSON.stringify(reconnect.result.value, null, 2)}`);
}

console.log("dashboard smoke passed");
console.log(value.chatSnapshot);
console.log(value.terminalModeSnapshot);
await cdp.close();

async function dashboardSmoke(input) {
  const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const waitFor = async (predicate, label, timeout = 10000) => {
    const deadline = Date.now() + timeout;
    while (Date.now() < deadline) {
      const value = await predicate();
      if (value) return value;
      await delay(100);
    }
    throw new Error("timed out waiting for " + label);
  };
  const setValue = (element, value) => {
    const setter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(element), "value").set;
    setter.call(element, value);
    element.dispatchEvent(new Event("input", { bubbles: true }));
  };
  const clickText = (text, root = document) => {
    const button = [...root.querySelectorAll("button")].find((item) => item.textContent.trim() === text);
    if (!button) throw new Error("missing button " + text);
    button.click();
    return button;
  };
  const snapshotText = async (sessionName) => {
    const list = await fetch("/api/v1/sessions", { credentials: "same-origin" }).then((response) => response.json());
    const session = list.sessions.find((item) => item.name === sessionName);
    if (!session) return "";
    const snapshot = await fetch("/api/v1/sessions/" + session.id + "/snapshot", { credentials: "same-origin" }).then((response) => response.json());
    return snapshot.chunks.map((chunk) => atob(chunk.bytes)).join("");
  };
  const createSession = async (nameValue, mode) => {
    const form = document.querySelector(".create-form");
    const [name, command, args, cwd] = [...form.querySelectorAll("input")];
    setValue(name, nameValue);
    setValue(command, "/bin/sh");
    setValue(args, "");
    setValue(cwd, "");
    clickText(mode === "terminal" ? "Terminal" : "Chat", form);
    await delay(100);
    form.requestSubmit();
  };

  try {
    const password = document.querySelector(".login-panel input[type=password]");
    if (password) {
      setValue(password, input.token);
      document.querySelector(".login-panel").requestSubmit();
    }

    await waitFor(() => document.querySelector(".create-form"), "dashboard");

    await createSession(input.chatSessionName, "chat");
    await waitFor(() => document.querySelector(".chat-view .composer textarea"), "chat composer");
    setValue(document.querySelector(".chat-view .composer textarea"), "echo chat:" + input.chatLine);
    await delay(150);
    clickText("Send", document.querySelector(".composer"));
    const chatSnapshot = await waitFor(async () => {
      const text = await snapshotText(input.chatSessionName);
      return text.includes("chat:" + input.chatLine) ? text : "";
    }, "chat output");
    if (!document.body.innerText.includes("chat:" + input.chatLine)) {
      throw new Error("chat transcript did not show command output");
    }
    clickText("/", document.querySelector(".composer"));
    await waitFor(() => document.querySelector(".slash-menu"), "slash menu");
    clickText("Refresh snapshot", document.querySelector(".slash-menu"));
    await waitFor(() => !document.querySelector(".slash-menu"), "slash menu closed");

    clickText("Open Terminal");
    await waitFor(() => document.querySelector(".terminal-section .raw-input textarea"), "terminal mode");
    if (!document.querySelector(".xterm-rows")) throw new Error("xterm rows missing after mode switch");
    setValue(document.querySelector(".terminal-section .raw-input textarea"), "echo terminal:" + input.terminalLine + String.fromCharCode(10));
    await delay(150);
    clickText("Send", document.querySelector(".raw-input"));
    await waitFor(async () => {
      const text = await snapshotText(input.chatSessionName);
      return text.includes("terminal:" + input.terminalLine) ? text : "";
    }, "terminal output");

    clickText("Open Chat");
    await waitFor(() => document.querySelector(".chat-view"), "chat mode restored");

    await createSession(input.terminalSessionName, "terminal");
    await waitFor(() => document.querySelector(".terminal-section .raw-input textarea"), "new terminal session");
    setValue(document.querySelector(".terminal-section .raw-input textarea"), "echo terminal-mode:" + input.terminalModeLine + String.fromCharCode(10));
    await delay(150);
    clickText("Send", document.querySelector(".raw-input"));
    const terminalModeSnapshot = await waitFor(async () => {
      const text = await snapshotText(input.terminalSessionName);
      return text.includes("terminal-mode:" + input.terminalModeLine) ? text : "";
    }, "terminal mode output");

    clickText("Interrupt");
    window.confirm = () => true;
    clickText("Terminate");
    await waitFor(() => document.body.innerText.includes("terminated") || document.body.innerText.includes("exited"), "terminate status", 10000);

    return { ok: true, chatSnapshot, terminalModeSnapshot, bodyText: document.body.innerText };
  } catch (err) {
    return {
      ok: false,
      error: err.message,
      href: location.href,
      readyState: document.readyState,
      title: document.title,
      rows: document.querySelector(".xterm-rows")?.innerText || document.querySelector(".xterm-rows")?.textContent || "",
      bodyText: document.body.innerText,
      html: document.documentElement.outerHTML.slice(0, 2400)
    };
  }
}

async function reconnectSmoke(input) {
  const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const waitFor = async (predicate, label, timeout = 10000) => {
    const deadline = Date.now() + timeout;
    while (Date.now() < deadline) {
      const value = await predicate();
      if (value) return value;
      await delay(100);
    }
    throw new Error("timed out waiting for " + label);
  };
  try {
    await waitFor(() => document.querySelector(".create-form"), "dashboard reconnect");
    const reconnectText = await waitFor(async () => {
      if (!document.body.innerText.includes(input.chatSessionName)) return "";
      const list = await fetch("/api/v1/sessions", { credentials: "same-origin" }).then((response) => response.json());
      const session = list.sessions.find((item) => item.name === input.chatSessionName);
      if (!session) return "";
      const snapshot = await fetch("/api/v1/sessions/" + session.id + "/snapshot", { credentials: "same-origin" }).then((response) => response.json());
      const text = snapshot.chunks.map((chunk) => atob(chunk.bytes)).join("");
      return text.includes("chat:" + input.chatLine) && text.includes("terminal:" + input.terminalLine) ? text : "";
    }, "reconnect snapshot");
    return { ok: true, reconnectText, bodyText: document.body.innerText };
  } catch (err) {
    return { ok: false, error: err.message, bodyText: document.body.innerText };
  }
}

function getJSON(url) {
  return new Promise((resolve, reject) => {
    fetch(url).then((response) => {
      if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
      return response.json();
    }).then(resolve, reject);
  });
}

function connectCDP(url) {
  const socket = new WebSocket(url);
  let nextID = 1;
  const pending = new Map();
  const handlers = new Map();

  socket.addEventListener("message", (event) => {
    const message = JSON.parse(event.data);
    if (!message.id) {
      handlers.get(message.method)?.(message.params || {});
      return;
    }
    const callbacks = pending.get(message.id);
    if (!callbacks) return;
    pending.delete(message.id);
    if (message.error) callbacks.reject(new Error(message.error.message));
    else callbacks.resolve(message.result);
  });

  return new Promise((resolve, reject) => {
    socket.addEventListener("open", () => {
      resolve({
        on(method, handler) {
          handlers.set(method, handler);
        },
        send(method, params = {}) {
          const id = nextID++;
          socket.send(JSON.stringify({ id, method, params }));
          return new Promise((resolveSend, rejectSend) => {
            pending.set(id, { resolve: resolveSend, reject: rejectSend });
          });
        },
        close() {
          socket.close();
        }
      });
    });
    socket.addEventListener("error", () => reject(new Error("CDP socket failed")));
  });
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
