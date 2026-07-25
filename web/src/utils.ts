import type { SessionStatus } from "./types";

export function splitArgs(value: string): string[] {
  return value.match(/(?:[^\s"]+|"[^"]*")+/g)?.map((part) => part.replace(/^"|"$/g, "")) || [];
}

export function decodeBase64(value: string): string {
  return atob(value);
}

export function encodeBase64(value: string): string {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

export function terminalOutputText(data: unknown): string {
  const payload = data as { data?: string; bytes?: string } | undefined;
  const encoded = payload?.bytes || payload?.data;
  return encoded ? decodeBase64(encoded) : "";
}

export function isLive(status: SessionStatus): boolean {
  return status === "starting" || status === "running";
}

export function isUnsafeMethod(method: string): boolean {
  return !["GET", "HEAD", "OPTIONS", "TRACE"].includes(method.toUpperCase());
}

export function commandLine(command: string, args: string[]): string {
  return [command, ...args].join(" ");
}

export function cleanTerminalText(value: string): string {
  return value
    .replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "")
    .replace(/\x1b\][^\x07]*(\x07|\x1b\\)/g, "")
    .replace(/\r/g, "")
    .replace(/\u001b/g, "")
    .trim();
}
