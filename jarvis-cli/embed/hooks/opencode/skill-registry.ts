/**
 * Jarvis — OpenCode project skill registry refresh plugin.
 * Runs quietly and non-fatally against the active project/worktree cwd.
 */

import { execFile } from "node:child_process"
import type { Plugin } from "@opencode-ai/plugin"

const JARVIS_EXECUTABLE = {{JARVIS_EXECUTABLE}}

function readString(value: unknown): string {
  return typeof value === "string" ? value.trim() : ""
}

function resolveDirectory(context: any, input: any): string {
  const fromContext = readString(context.worktree) || readString(context.directory) || readString(context.cwd) ||
    readString(context.workspace?.directory)
  if (fromContext) return fromContext

  const fromInput = readString(input.worktree) || readString(input.directory) || readString(input.cwd) ||
    readString(input.workspace?.directory)
  if (fromInput) return fromInput

  const envDirectory = readString(process.env["HIVE_PROJECT_DIRECTORY"]) ||
    readString(process.env["JARVIS_WORKSPACE_DIRECTORY"])
  if (envDirectory) return envDirectory

  try {
    const cwd = process.cwd()
    if (cwd) return cwd
  } catch {
    // Fall through to PWD as the last-resort compatibility fallback.
  }
  return readString(process.env["PWD"])
}

function refreshSkillRegistry(directory: string): void {
  if (!directory) {
    console.error("Project skill registry warning: active project directory unavailable")
    return
  }

  const child = execFile(
    JARVIS_EXECUTABLE,
    ["skill-registry", "refresh", "--quiet", "--cwd", directory],
    { timeout: {{JARVIS_REFRESH_TIMEOUT_MILLIS}} },
    (error, _stdout, stderr) => {
      if (error) {
        console.error(`Project skill registry warning: refresh failed for ${directory}`)
      }
      const warning = stderr.trim()
      if (warning) {
        for (const line of warning.split(/\r?\n/)) {
          if (line.trim()) console.error(`Project skill registry warning: ${line.trim()}`)
        }
      }
    },
  )
  child.on("error", () => {
    console.error("Project skill registry warning: jarvis executable unavailable")
  })
}

export const SkillRegistry: Plugin = async (context: any = {}) => {
  let refreshed = false
  return {
    event: async (input: any) => {
      if (refreshed) return
      refreshed = true
      refreshSkillRegistry(resolveDirectory(context ?? {}, input ?? {}))
    },
    "chat.message": async (input: any) => {
      if (refreshed) return
      refreshed = true
      refreshSkillRegistry(resolveDirectory(context ?? {}, input ?? {}))
    },
  }
}
