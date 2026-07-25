const baseURL = process.env.HARNESSRELAY_DASHBOARD_URL || "http://127.0.0.1:8767/";
const token = process.env.HARNESSRELAY_TOKEN || "dashboard-token";
const cdpURL = process.env.CHROME_CDP_URL || "http://127.0.0.1:9222/json/list";
const sessionName = `dash-smoke-${Date.now()}`;
const inputLine = `dashboard smoke ${Date.now()}`;

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
  expression: `(${async function dashboardSmoke(input) {
    const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
    const waitFor = async (predicate, label, timeout = 8000) => {
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

    try {
      const password = document.querySelector(".login-panel input[type=password]");
      if (password) {
        setValue(password, input.token);
        document.querySelector(".login-panel").requestSubmit();
      }

      await waitFor(() => document.querySelector(".create-form"), "dashboard");
      const [name, command, args] = [...document.querySelectorAll(".create-form input")];
      setValue(name, input.sessionName);
      setValue(command, "/bin/sh");
      setValue(args, "testdata/fake-harnesses/interactive-echo.sh");
      await delay(100);
      document.querySelector(".create-form").requestSubmit();

      await waitFor(() => document.querySelector(".raw-input textarea"), "raw input");
      const textarea = document.querySelector(".raw-input textarea");
      setValue(textarea, input.inputLine + String.fromCharCode(10));
      await delay(100);
      document.querySelector(".raw-input button").click();

      const snapshotText = await waitFor(async () => {
        const list = await fetch("/api/v1/sessions", { credentials: "same-origin" }).then((response) => response.json());
        const session = list.sessions.find((item) => item.name === input.sessionName);
        if (!session) return "";
        const snapshot = await fetch("/api/v1/sessions/" + session.id + "/snapshot", { credentials: "same-origin" }).then((response) => response.json());
        const text = snapshot.chunks.map((chunk) => atob(chunk.bytes)).join("");
        return text.includes("echo:" + input.inputLine) ? text : "";
      }, "snapshot echo", 10000);

      const rows = (() => {
        const node = document.querySelector(".xterm-rows");
        return node?.innerText || node?.textContent || "";
      })();

      return { ok: true, rows, snapshotText, bodyText: document.body.innerText };
    } catch (err) {
      return {
        ok: false,
        error: err.message,
        href: location.href,
        readyState: document.readyState,
        title: document.title,
        rows: document.querySelector(".xterm-rows")?.innerText || document.querySelector(".xterm-rows")?.textContent || "",
        bodyText: document.body.innerText,
        html: document.documentElement.outerHTML.slice(0, 2000)
      };
    }
  }})(${JSON.stringify({ token, sessionName, inputLine })})`
});

const value = result.result.value;
if (!value?.ok || !value.snapshotText?.includes(`echo:${inputLine}`)) {
  if (diagnostics.length > 0) value.diagnostics = diagnostics;
  throw new Error(`dashboard smoke failed: ${JSON.stringify(value, null, 2)}`);
}

await cdp.send("Page.navigate", { url: baseURL });
await delay(500);
const reconnect = await cdp.send("Runtime.evaluate", {
  awaitPromise: true,
  returnByValue: true,
  expression: `(${async function reconnectSmoke(input) {
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
        if (!document.body.innerText.includes(input.sessionName)) return "";
        const list = await fetch("/api/v1/sessions", { credentials: "same-origin" }).then((response) => response.json());
        const session = list.sessions.find((item) => item.name === input.sessionName);
        if (!session) return "";
        const snapshot = await fetch("/api/v1/sessions/" + session.id + "/snapshot", { credentials: "same-origin" }).then((response) => response.json());
        const text = snapshot.chunks.map((chunk) => atob(chunk.bytes)).join("");
        return text.includes("echo:" + input.inputLine) ? text : "";
      }, "reconnect snapshot");
      return { ok: true, reconnectText, bodyText: document.body.innerText };
    } catch (err) {
      return { ok: false, error: err.message, bodyText: document.body.innerText };
    }
  }})(${JSON.stringify({ sessionName, inputLine })})`
});
if (!reconnect.result.value?.ok || !reconnect.result.value.reconnectText?.includes(`echo:${inputLine}`)) {
  throw new Error(`dashboard reconnect failed: ${JSON.stringify(reconnect.result.value, null, 2)}`);
}
console.log("dashboard smoke passed");
console.log(value.snapshotText);
await cdp.close();

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
