# Security

## Recommendation

Treat the daemon as a local command-control surface. Bind to `127.0.0.1` by default, require authentication for dashboard/API/WebSocket even on localhost, use CSRF protection for state-changing REST requests, validate WebSocket `Origin`, audit sensitive actions, redact secrets, and refuse to run as root by default.

## Alternatives Considered

| Alternative | Decision | Reason |
| --- | --- | --- |
| No auth on localhost | Reject | Any browser origin can attempt localhost requests; the daemon controls terminal sessions. |
| Bearer token only in localStorage | Reject as default | Easier for APIs, but localStorage increases exposure if the dashboard has XSS. |
| HTTP basic auth only | Defer to development fallback | Simple, but weaker UX for CSRF-aware browser sessions and token rotation. |
| Bind `0.0.0.0` by default | Reject | Public or LAN exposure must be an explicit, authenticated, warned configuration. |
| Allow root daemon | Reject by default | Harnesses inherit privileges and a dashboard compromise would become root command control. |

## Threat Model

The daemon can:

- type into local shells and coding harnesses
- trigger file edits through harnesses
- run commands indirectly
- expose terminal output that may include secrets
- hold session history and audit records

Therefore, an attacker who reaches the dashboard/API can potentially control local development environments.

## Safe Defaults

| Setting | Default |
| --- | --- |
| Bind address | `127.0.0.1` |
| Port | Project-configured default in Phase 1; must not require privileged port. |
| Authentication | Required. |
| WebSocket auth | Required at handshake. |
| CSRF | Required for cookie-auth state changes. |
| CORS | Disabled by default except same-origin. |
| Public bind | Disabled by default. |
| Run as root | Refuse unless explicit unsafe override for development/testing. |
| Audit logging | Enabled for session control and semantic actions. |
| Secret logging | Redacted/avoid payload logging by default. |

## Authentication Approach

Stage 1 recommended approach:

- On first run, generate a high-entropy local access token or require the user to set a password.
- Store token/password verifier in user-only config storage with permissions `0600`.
- Use an HttpOnly, SameSite cookie after login for dashboard sessions.
- Also support `Authorization: Bearer <token>` for CLI/testing.
- Do not print full tokens repeatedly in logs. If a first-run token is shown, show once and mark it sensitive.

Basic auth is acceptable only as a development fallback. OpenCode documents `OPENCODE_SERVER_PASSWORD` for HTTP basic auth on its local server, but HarnessRelay should prefer session cookies plus CSRF for browser use.

## CSRF Handling

If browser auth uses cookies, every state-changing REST endpoint must require a CSRF token/header.

Recommended:

- Session cookie: `HttpOnly`, `SameSite=Strict` for localhost dashboard.
- CSRF token: issued by authenticated bootstrap endpoint or embedded in initial HTML response.
- Client sends `X-CSRF-Token` on `POST`, `PUT`, `PATCH`, and `DELETE`.
- Reject state-changing requests with missing/invalid CSRF token.
- Validate `Origin` or `Referer` against configured dashboard origin.
- Add Fetch Metadata checks where available, rejecting cross-site unsafe requests.

OWASP notes that CSRF targets state-changing actions and recommends CSRF tokens, custom headers, and Origin/Referer validation depending on app shape.

## WebSocket Authentication And Origin

WebSockets do not provide authentication by themselves. The handshake must validate:

- authenticated session cookie or explicit short-lived WebSocket token
- `Origin` exactly matches allowed dashboard origins
- requested session filters are authorized

Recommended WebSocket URL:

```text
GET /api/v1/ws?after_seq=123
```

Do not put long-lived bearer tokens in query strings because URLs can appear in logs. Prefer authenticated cookie plus CSRF/nonce-derived short-lived WS token, or use the `Sec-WebSocket-Protocol` header if the chosen Go library/client support is clean.

WebSocket server requirements:

- explicit Origin allowlist
- read size limits
- ping/pong heartbeat
- per-connection outbound buffer bounds
- close connection on auth expiry/logout
- no `CheckOrigin: return true`

## Non-Local Bind Rules

Binding to anything other than localhost must require all of:

- explicit config value, not command typo
- authentication configured
- CSRF and WebSocket Origin allowlist configured
- startup warning that remote exposure can control local terminal sessions
- audit log entry on startup
- documentation recommending private network/VPN/SSH tunnel over public exposure

Reject `0.0.0.0` if auth is disabled or default token is not initialized.

## Audit Logging

Audit these events:

- login success/failure
- session create
- terminal input metadata only: byte count, session, actor, timestamp
- resize metadata
- interrupt
- terminate/force kill
- semantic action shown
- semantic action accepted/rejected/stale
- non-local bind startup
- auth/config changes

Do not log:

- raw terminal input payloads by default
- full terminal output by default in audit log
- cookies, bearer tokens, CSRF tokens
- environment variable values matching secret patterns

## Secret Redaction

Add a shared redaction helper in later phases. Redact keys/names containing:

- `TOKEN`
- `SECRET`
- `PASSWORD`
- `PASS`
- `API_KEY`
- `PRIVATE_KEY`
- `SSH_AUTH_SOCK` value if exposed in logs

Terminal history may still contain secrets typed or printed by user tools. Document this and keep history bounded.

## Root Execution Rule

The daemon must not run as root by default.

Reasoning:

- Harnesses inherit daemon privileges.
- A compromised dashboard would become root command execution.
- Root-owned PTY/session files and config can create cleanup and permission hazards.

Implementation guidance:

- On Linux, if `os.Geteuid() == 0`, fail startup with a clear message.
- Allow only an explicit `--allow-root-for-testing` or config flag marked unsafe, and log a warning.

## Risks Of Public Exposure

Public exposure can allow:

- command execution through terminal input
- source-code and secret disclosure through terminal output/history
- cross-site WebSocket hijacking if Origin checks are weak
- CSRF-driven session control if cookie auth lacks CSRF
- denial of service through PTY/session spawning or WebSocket flooding
- accidental approvals in coding harnesses

Stage 1 should document remote use as advanced and prefer SSH tunnel or private networking.

## Acceptance Criteria For Later Implementation

- Daemon binds to `127.0.0.1` by default.
- Unauthenticated REST, dashboard, and WebSocket access fails.
- State-changing REST without CSRF token fails.
- WebSocket from unexpected Origin fails.
- Non-local bind without explicit auth/config fails.
- Running as root fails by default.
- Audit log records session control and semantic actions without sensitive payloads.
- Stale approval action rejection is security-tested.

## Required Tests

- Default config bind address is `127.0.0.1`.
- API rejects unauthenticated session creation.
- WebSocket rejects unauthenticated handshake.
- WebSocket rejects malicious Origin.
- CSRF missing/wrong token rejected for POST.
- SameSite/HttpOnly cookie attributes set.
- Non-local bind requires explicit config and auth.
- Root check can be unit-tested behind injectable `geteuid`.
- Logs redact common secret env names.
- Stale action returns `409` and audit event.

## Sources

- [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
- [OWASP WebSocket Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html)
- [OWASP HTML5 Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/HTML5_Security_Cheat_Sheet.html)
- [OpenCode server authentication docs](https://dev.opencode.ai/docs/server/)
