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

interface LifecycleEvidence {
  project: string;
  directory: string;
}

interface LifecycleNotification extends LifecycleEvidence {
  id: string;
}

interface PromptPart {
  type?: unknown;
  text?: unknown;
}

interface PromptOutput {
  parts?: PromptPart[];
}

async function reportMigrationStatus(): Promise<void> {
  try {
    const response = await fetch(
      `http://127.0.0.1:${HIVE_PORT}/governance/project-identity/status`,
      { signal: AbortSignal.timeout(1000) },
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

// Prompt resolution intentionally retains the pre-lifecycle compatibility order.
function resolveHiveSessionId(input: unknown, output: unknown): string {
  const envSession =
    readString(process.env["HIVE_OPENCODE_SESSION_ID"]) ||
    readString(process.env["OPENCODE_SESSION_ID"]) ||
    readString(process.env["SESSION_ID"]);
  if (envSession) return envSession;

  const session = readPath(input, output, [
    "session_id",
    "sessionId",
    "sessionID",
  ]);
  if (session) return session;

  return `ppid-${process.ppid ?? process.pid}`;
}

function resolveHiveDirectory(input: unknown, output: unknown): string {
  const envDirectory =
    readString(process.env["HIVE_PROJECT_DIRECTORY"]) ||
    readString(process.env["JARVIS_WORKSPACE_DIRECTORY"]) ||
    readString(process.env["PWD"]);
  if (envDirectory) return envDirectory;

  const directory = readPath(input, output, ["directory", "cwd", "workspace"]);
  if (directory) return directory;

  try {
    return process.cwd();
  } catch {
    return "";
  }
}

function resolveHiveProject(input: unknown, output: unknown): string {
  const envProject =
    readString(process.env["HIVE_PROJECT"]) || readString(process.env["JARVIS_PROJECT"]);
  if (envProject) return envProject;
  return readPath(input, output, ["project", "projectName"]);
}

function resolveLifecycleSessionId(event: unknown): string {
  const properties = readRecord(readRecord(event)["properties"]);
  const info = readRecord(properties["info"]);
  const documentedID = readString(info["id"]);
  if (documentedID) return documentedID;

  const lifecycleID = readPath(properties, {}, [
    "session_id",
    "sessionId",
    "sessionID",
  ]);
  if (lifecycleID) return lifecycleID;

  return (
    readString(process.env["HIVE_OPENCODE_SESSION_ID"]) ||
    readString(process.env["OPENCODE_SESSION_ID"]) ||
    readString(process.env["SESSION_ID"])
  );
}

function resolveLifecycleEvidence(event: unknown): LifecycleEvidence {
  const properties = readRecord(readRecord(event)["properties"]);
  const info = readRecord(properties["info"]);
  return {
    project: resolveHiveProject(info, properties),
    directory: resolveHiveDirectory(info, properties),
  };
}

function resolveLifecycleNotification(event: unknown): LifecycleNotification | undefined {
  const id = resolveLifecycleSessionId(event);
  const evidence = resolveLifecycleEvidence(event);
  if (!id || (!evidence.project && !evidence.directory)) return undefined;
  return { id, ...evidence };
}

function lifecyclePayload(notification: LifecycleNotification, includeID: boolean): Record<string, string> {
  const payload: Record<string, string> = { client: HIVE_CLIENT };
  if (includeID) payload.id = notification.id;
  if (notification.project) payload.project = notification.project;
  if (notification.directory) payload.directory = notification.directory;
  return payload;
}

async function deliverLifecycle(url: string, payload: Record<string, string>): Promise<void> {
  try {
    const response = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
      signal: AbortSignal.timeout(1000),
    });
    if (!response.ok) {
      console.warn(`Hive lifecycle delivery rejected: endpoint=${url} status=${response.status}`);
    }
  } catch {
    // Lifecycle delivery is advisory and must not block OpenCode.
  }
}

function registerLifecycleOnce(notification: LifecycleNotification): Promise<void> {
  const key = JSON.stringify([
    notification.id,
    "evidence",
    notification.project,
    notification.directory,
  ]);
  const existing = createdFlights.get(key);
  if (existing) return existing;

  let flight: Promise<void>;
  flight = Promise.resolve()
    .then(() =>
      deliverLifecycle(HIVE_ENDPOINTS.SESSION_START, lifecyclePayload(notification, true)),
    )
    .catch(() => undefined)
    .finally(() => {
      if (createdFlights.get(key) === flight) createdFlights.delete(key);
    });
  createdFlights.set(key, flight);
  return flight;
}

async function notifySessionCreated(event: unknown): Promise<void> {
  try {
    const notification = resolveLifecycleNotification(event);
    if (notification) await registerLifecycleOnce(notification);
  } catch {
    // Event decoding is advisory and must not block OpenCode.
  }
}

async function notifySessionDeleted(event: unknown): Promise<void> {
  try {
    const notification = resolveLifecycleNotification(event);
    if (!notification) return;
    await deliverLifecycle(
      `${HIVE_URL}/sessions/${encodeURIComponent(notification.id)}/end`,
      lifecyclePayload(notification, false),
    );
  } catch {
    // Event decoding is advisory and must not block OpenCode.
  }
}

export const Hive: Plugin = async () => {
  await reportMigrationStatus();
  return {
    event: ({ event }) => {
      if (event.type === "session.created") {
        void notifySessionCreated(event);
      } else if (event.type === "session.deleted") {
        void notifySessionDeleted(event);
      }
    },
    "chat.message": async (input: unknown, output: unknown) => {
      try {
        const parts = (output as PromptOutput | null | undefined)?.parts ?? [];
        const content = parts
          .filter((part) => part?.type === "text")
          .map((part) => part?.text ?? "")
          .join("\n")
          .trim();

        if (!content) return;

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
