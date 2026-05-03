/**
 * Hive — OpenCode plugin
 * Captures user prompts and POSTs them to the local hive-daemon HTTP server.
 * Fire-and-forget: never blocks the user.
 */

import type { Plugin } from "@opencode-ai/plugin"

const HIVE_PORT = parseInt(process.env["HIVE_HTTP_PORT"] ?? "7438", 10)
const HIVE_URL = `http://127.0.0.1:${HIVE_PORT}/prompts`

export const Hive: Plugin = async () => {
  return {
    "chat.message": async (_input: unknown, output: any) => {
      const content = (output?.parts ?? [])
        .filter((p: any) => p?.type === "text")
        .map((p: any) => p?.text ?? "")
        .join("\n")
        .trim()

      if (!content) return

      try {
        await fetch(HIVE_URL, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ content }),
          signal: AbortSignal.timeout(1000),
        })
      } catch {
        // Daemon not running, timeout, or any error — silently ignore.
      }
    },
  }
}
