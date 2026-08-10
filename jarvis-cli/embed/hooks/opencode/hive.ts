/**
 * Hive — OpenCode plugin
 * Captures user prompts and POSTs them to the local hive-daemon HTTP server.
 * Fire-and-forget: never blocks the user.
 */

import type { Plugin } from "@opencode-ai/plugin"

const HIVE_PORT = parseInt(process.env["HIVE_HTTP_PORT"] ?? "7438", 10)
const HIVE_URL = `http://127.0.0.1:${HIVE_PORT}/prompts`

async function reportMigrationStatus(): Promise<void> {
  try {
    const response = await fetch(`http://127.0.0.1:${HIVE_PORT}/governance/project-identity/status`, {
      signal: AbortSignal.timeout(1000),
    })
    const status = await response.json()
    if (status?.state === "migration-blocked") {
      console.warn(`Hive migration-blocked: ${status.reason ?? "unknown reason"}. Backup: ${status.backup_id ?? "unavailable"}. Continue with: hive project identity status`)
    }
  } catch {
    // Startup status is advisory and must not block OpenCode.
  }
}

function readString(value: unknown): string {
  return typeof value === "string" ? value.trim() : ""
}

function readPath(input: any, output: any, keys: string[]): string {
  for (const key of keys) {
    const value = readString(input?.[key]) || readString(output?.[key])
    if (value) return value
  }
  return ""
}

function resolveHiveSessionId(input: unknown, output: unknown): string {
  const envSession = readString(process.env["HIVE_OPENCODE_SESSION_ID"]) ||
    readString(process.env["OPENCODE_SESSION_ID"]) ||
    readString(process.env["SESSION_ID"])
  if (envSession) return envSession

  const session = readPath(input as any, output as any, ["session_id", "sessionId", "sessionID"])
  if (session) return session

  return `ppid-${process.ppid ?? process.pid}`
}

function resolveHiveDirectory(input: unknown, output: unknown): string {
  const envDirectory = readString(process.env["HIVE_PROJECT_DIRECTORY"]) ||
    readString(process.env["JARVIS_WORKSPACE_DIRECTORY"]) ||
    readString(process.env["PWD"])
  if (envDirectory) return envDirectory

  const directory = readPath(input as any, output as any, ["directory", "cwd", "workspace"])
  if (directory) return directory

  try {
    return process.cwd()
  } catch {
    return ""
  }
}

function resolveHiveProject(input: unknown, output: unknown): string {
  const envProject = readString(process.env["HIVE_PROJECT"]) || readString(process.env["JARVIS_PROJECT"])
  if (envProject) return envProject
  return readPath(input as any, output as any, ["project", "projectName"])
}

export const Hive: Plugin = async () => {
  await reportMigrationStatus()
  return {
    "chat.message": async (input: unknown, output: any) => {
      const content = (output?.parts ?? [])
        .filter((p: any) => p?.type === "text")
        .map((p: any) => p?.text ?? "")
        .join("\n")
        .trim()

      if (!content) return

      try {
        const sessionId = resolveHiveSessionId(input, output)
        const directory = resolveHiveDirectory(input, output)
        const project = resolveHiveProject(input, output)
        const payload: Record<string, string> = { content, session_id: sessionId }
        if (directory) payload.directory = directory
        if (project) payload.project = project

        await fetch(HIVE_URL, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
          signal: AbortSignal.timeout(1000),
        })
      } catch {
        // Daemon not running, timeout, or any error — silently ignore.
      }
    },
  }
}
