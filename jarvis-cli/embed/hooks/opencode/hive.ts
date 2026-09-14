/**
 * Hive — OpenCode plugin
 * Captures user prompts and POSTs them to the local hive-daemon HTTP server.
 * Fire-and-forget: never blocks the user.
 */

import type { Plugin } from "@opencode-ai/plugin";

const HIVE_PORT = parseInt(process.env["HIVE_HTTP_PORT"] ?? "7438", 10);
const HIVE_URL = `http://127.0.0.1:${HIVE_PORT}`;
const HIVE_CLIENT = "opencode";
const HIVE_ENDPOINTS = {
  PROMPTS: `${HIVE_URL}/prompts`,
  SESSION_START: `${HIVE_URL}/sessions`,
} as const;
const createdFlights = new Map<string, Promise<void>>();

async function reportMigrationStatus(): Promise<void> {
  try {
    const response = await fetch(
      `http://127.0.0.1:${HIVE_PORT}/governance/project-identity/status`,
      {
        signal: AbortSignal.timeout(1000),
      },
    );
    const status = await response.json();
    if (status?.state === "migration-blocked") {
      console.warn(
        `Hive migration-blocked: ${status.reason ?? "unknown reason"}. Backup: ${status.backup_id ?? "unavailable"}. Continue with: hive project identity status`,
      );
    }
  } catch {
    // Startup status is advisory and must not block OpenCode.
  }
}

function readString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function readText(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function readRecord(value: unknown): Record<string, unknown> {
  return typeof value === "object" && value !== null
    ? (value as Record<string, unknown>)
    : {};
}

function readPath(input: unknown, output: unknown, keys: readonly string[]): string {
  const inputRecord = readRecord(input);
  const outputRecord = readRecord(output);
  for (const key of keys) {
    const value = readString(inputRecord[key]) || readString(outputRecord[key]);
    if (value) return value;
  }
  return "";
}

function resolveHiveSessionId(input: unknown, output: unknown): string {
  const session = readPath(input, output, [
    "id",
    "session_id",
    "sessionId",
    "sessionID",
  ]);
  if (session) return session;

  return (
    readString(process.env["HIVE_OPENCODE_SESSION_ID"]) ||
    readString(process.env["OPENCODE_SESSION_ID"]) ||
    readString(process.env["SESSION_ID"])
  );
}

function resolveHiveDirectory(input: unknown, output: unknown): string {
  const directory = readPath(input, output, ["directory", "cwd", "workspace"]);
  if (directory) return directory;

  return (
    readString(process.env["HIVE_PROJECT_DIRECTORY"]) ||
    readString(process.env["JARVIS_WORKSPACE_DIRECTORY"]) ||
    readString(process.env["PWD"])
  );
}

function resolveHiveProject(input: unknown, output: unknown): string {
  const project = readPath(input, output, ["project", "projectName"]);
  if (project) return project;
  return (
    readString(process.env["HIVE_PROJECT"]) ||
    readString(process.env["JARVIS_PROJECT"])
  );
}

function resolveHiveDeveloperID(): string {
  return (
    readString(process.env["HIVE_DEV_ID"]) ||
    readString(process.env["JARVIS_DEV_ID"]) ||
    readString(process.env["USER"]) ||
    readString(process.env["USERNAME"]) ||
    "unknown"
  );
}

function notifySessionDeleted(input: unknown, output: unknown): void {
  const id = readPath(input, output, ["id", "session_id", "sessionId", "sessionID"]);
  if (!id) return;

  const directory = resolveHiveDirectory(input, output);
  const project = resolveHiveProject(input, output);
  const summary = readPath(input, output, ["summary"]);
  const payload: Record<string, string> = { client: HIVE_CLIENT };
  if (directory) payload.directory = directory;
  if (project) payload.project = project;
  if (summary) payload.summary = summary;

  let request: Promise<Response>;
  try {
    request = fetch(`${HIVE_URL}/sessions/${encodeURIComponent(id)}/end`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
      signal: AbortSignal.timeout(1000),
    });
  } catch {
    return;
  }
  void request.catch(() => undefined);
}

function notifySessionCreated(input: unknown, output: unknown): void {
  const id = resolveHiveSessionId(input, output);
  if (!id || createdFlights.has(id)) return;

  const directory = resolveHiveDirectory(input, output);
  const project = resolveHiveProject(input, output);
  const payload: Record<string, string> = {
    id,
    dev_id: resolveHiveDeveloperID(),
    client: HIVE_CLIENT,
  };
  if (directory) payload.directory = directory;
  if (project) payload.project = project;

  let request: Promise<Response>;
  try {
    request = fetch(HIVE_ENDPOINTS.SESSION_START, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
      signal: AbortSignal.timeout(1000),
    });
  } catch {
    return;
  }
  const flight = request
    .catch(() => undefined)
    .then(() => undefined)
    .finally(() => {
      createdFlights.delete(id);
    });
  createdFlights.set(id, flight);
}

export const Hive: Plugin = async () => {
  await reportMigrationStatus();
  return {
    event: ({ event }) => {
      if (event.type === "session.created") {
        notifySessionCreated(event.properties, {});
      } else if (event.type === "session.deleted") {
        const properties = readRecord(event.properties);
        notifySessionDeleted(properties["info"], event.properties);
      }
    },
    "chat.message": async (input: unknown, output: unknown) => {
      const parts = readRecord(output)["parts"];
      const content = (Array.isArray(parts) ? parts : [])
        .map(readRecord)
        .filter((part) => part["type"] === "text")
        .map((part) => readText(part["text"]))
        .join("\n")
        .trim();

      if (!content) return;

      try {
        const sessionId = resolveHiveSessionId(input, output);
        const directory = resolveHiveDirectory(input, output);
        const project = resolveHiveProject(input, output);
        const payload: Record<string, string> = {
          content,
          session_id: sessionId,
          client: HIVE_CLIENT,
        };
        if (directory) payload.directory = directory;
        if (project) payload.project = project;

        const response = await fetch(HIVE_ENDPOINTS.PROMPTS, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
          signal: AbortSignal.timeout(1000),
        });
        if (!response.ok) {
          console.warn(
            `Hive prompt capture rejected: endpoint=${HIVE_ENDPOINTS.PROMPTS} status=${response.status}`,
          );
        }
      } catch {
        // Daemon not running, timeout, or any error — silently ignore.
      }
    },
  };
};
