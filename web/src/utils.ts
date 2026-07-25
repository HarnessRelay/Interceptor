import type { SessionStatus } from "./types";

export function splitArgs(value: string): string[] {
  return value.match(/(?:[^\s"]+|"[^"]*")+/g)?.map((part) => part.replace(/^"|"$/g, "")) || [];
}

export function decodeBase64(value: string): string {
  return new TextDecoder().decode(decodeBase64Bytes(value));
}

export function decodeBase64Bytes(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
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
    .replace(/\x1b[()][0-2AB]/g, "")
    .replace(/\r/g, "")
    .replace(/\u001b/g, "")
    .replace(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/g, "")
    .trim();
}

export type ChatProjection = {
  kind: "readable" | "terminal";
  text: string;
};

const terminalStatusMessage = "Terminal interface output is available in Terminal Mode. Chat Mode could not convert this PTY redraw into readable chat.";
const boxDrawingPattern = /[┌┐└┘├┤┬┴┼─│╭╮╰╯═║╔╗╚╝╠╣╦╩╬]/;
const mojibakePattern = /(?:â[\u0080-\u00bf]?|ã[\u0080-\u00bf]?|�|□|â□)/;

export function projectTerminalOutputForChat(raw: string): ChatProjection {
  const text = cleanTerminalText(raw);
  if (!text) return { kind: "readable", text: "" };
  if (looksLikeTerminalRedraw(raw, text)) {
    return { kind: "terminal", text: terminalStatusMessage };
  }
  return { kind: "readable", text };
}

function looksLikeTerminalRedraw(raw: string, text: string): boolean {
  if (/\x1b\[\?1049[hl]/.test(raw) || /\x1b\[\?25[hl]/.test(raw)) return true;
  if (boxDrawingPattern.test(text) || mojibakePattern.test(text)) return true;
  if (looksLikeRepeatedArtifact(text)) return true;

  const controlMatches = raw.match(/\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\)|[()][0-2AB])/g) || [];
  const redrawControls = raw.match(/\x1b\[[0-?]*(?:[HJKST]|[0-9]+;[0-9]+[Hf])/g) || [];
  return controlMatches.length >= 8 && redrawControls.length >= 2;
}

function looksLikeRepeatedArtifact(text: string): boolean {
  const lines = text.split(/\n+/).map((line) => line.trim()).filter(Boolean);
  if (lines.length === 0 || lines.length > 4) return false;
  return lines.every((line) => {
    if (line.length < 6 || line.length > 80) return false;
    const chars = Array.from(line);
    if (!chars.every((char) => char === chars[0])) return false;
    return /^[A-Z0-9#_=~*.\-]+$/.test(line) || line.length <= 12;
  });
}
