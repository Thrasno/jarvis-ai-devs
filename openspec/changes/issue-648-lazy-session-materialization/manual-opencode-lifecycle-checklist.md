# OpenCode lifecycle manual checklist

## Current result

**Manual OpenCode runtime: NOT RUN.** This artifact records the required disposable loopback procedure and separates it from the source-derived Node observations already recorded for the current lifecycle test. No OpenCode model, authentication flow, or user configuration was invoked or changed for this record.

## Source of truth and disposable setup

The source of truth is `jarvis-cli/embed/hooks/opencode/hive.ts`. Materialize it only through `OpenCodeAgent.InstallPromptHook` into a temporary home; its installer target is `<temporary-home>/.config/opencode/plugins/hive.ts`. Do not copy, hand-edit, or inspect a developer's installed plugin, and do not modify `opencode.json`.

1. Create a disposable temporary home and a local HTTP loopback receiver on `127.0.0.1`.
2. Invoke the existing installer path with that temporary home and the embedded source filesystem.
3. Assert the installed `plugins/hive.ts` is byte-identical to `embed/hooks/opencode/hive.ts` before starting OpenCode.
4. Start only the local receiver and OpenCode with `HOME`/XDG configuration directed to the temporary home and `HIVE_HTTP_PORT` directed to the receiver. Do not authenticate or submit a request to an external model.
5. Record OpenCode version, operating system, runtime command, temporary-home path, and receiver log. Stop both processes and retain no user configuration changes.

## Manual loopback steps

Use one receiver log entry per request: method, path, raw body, `Content-Type`, timestamp, and selected response. Do not log prompt content outside the disposable test record.

1. Emit/create a session with project and directory evidence. Confirm the created request path, JSON body, and `client: "opencode"`.
2. While its created request is held pending, emit/delete a different session with project, directory, and summary. Confirm deletion arrives independently.
3. Use an ID containing a space, slash, question mark, and percent sign. Confirm the end path is percent-encoded and the body carries only applicable evidence, summary, and `client`.
4. Send a text prompt for the session. Confirm normal prompt capture remains usable and includes `session_id`, project/directory evidence when available, and `client: "opencode"`.
5. Return 400, 423, and 500 separately for created, deleted, and prompt requests. Confirm OpenCode remains usable and the plugin does not turn a rejected response into a visible failure.
6. Hold each endpoint longer than one second. Confirm the callback remains non-blocking, the loopback request is aborted or otherwise finishes fail-open, and no unrequested retry occurs.
7. Force synchronous `fetch` setup failure for each applicable path. Confirm the event/prompt callback returns fail-open.
8. After each failure scenario, inspect receiver logs and process list: no retry worker, unhandled failure, daemon launch, or extra process may be created by the plugin.

## Automated observations already proven

These are source-derived Node loopback observations from `jarvis-cli/internal/agent/opencode_lifecycle_test.go`, as recorded in the current slice evidence. They are not a live OpenCode-runtime result.

| Scenario | Status | Observed contract |
| --- | --- | --- |
| Created path, method, header, body, and client | PASS | `POST /sessions`, `Content-Type: application/json`, and exactly `id`, `project`, `directory`, `dev_id`, and `client: "opencode"` for the evidenced `session-42` request. |
| Created pending-flight behavior | PASS | Two immediate identical created callbacks produce no second request while the first is held; after the first request times out and a later created event is emitted, exactly two starts are observed. |
| Created timeout beyond one second | PASS | The loopback handler observes the first request context cancellation; the test waits 1100 ms before emitting the later event and verifies the later start is not observed before cancellation. |
| Created synchronous fetch setup failure | PASS | The runner makes `fetch` throw synchronously for a created event; the Node runner completes. |
| Prompt path, method, header, body, client, and independence | PASS | While created is pending, `POST /prompts` is observed with JSON `content`, `session_id`, project, directory, and `client: "opencode"`; multipart text is `capture  \n  this prompt`. |
| Deleted path, method, header, body, and client | PASS | Generic `session.deleted` with `properties.info` sends `POST /sessions/<encoded-id>/end`, JSON project/directory/summary/client, and no `dev_id`. |
| Encoded deletion ID | PASS | `session /with?reserved%chars` becomes `/sessions/session%20%2Fwith%3Freserved%25chars/end`. |
| Deleted independence from pending created | PASS | A held `created-pending` start request does not prevent the deletion end request. |
| Deleted timeout beyond one second | PASS | The first end request is held until context cancellation; after 1100 ms a later deleted event causes the second observed end, after the first cancellation. |
| Deleted synchronous fetch setup failure and missing-ID no-op | PASS | The runner makes `fetch` throw synchronously for a deletion event and then emits a deleted event with no ID; the Node runner completes and no end request is observed for the missing ID. |

## Explicitly unexecuted or not proven

| Scenario | Status | Rationale |
| --- | --- | --- |
| Live OpenCode envelope and runtime compatibility | NOT RUN | The current evidence imports the source template in Node; it does not start OpenCode. This record must not claim compatibility with `opencode 1.18.29` until the disposable procedure is run. |
| 400 response tolerance | NOT RUN | The lifecycle loopback handlers return successful responses; no 400 case is exercised. |
| 423 response tolerance | NOT RUN | The lifecycle loopback handlers return successful responses; no 423 case is exercised. |
| 500 response tolerance | NOT RUN | The lifecycle loopback handlers return successful responses; no 500 case is exercised. |
| Generic non-OK response tolerance | NOT RUN | No lifecycle test case returns a non-OK response. |
| Prompt timeout beyond one second | NOT RUN | Only created and deleted handlers are deliberately held until their request contexts are cancelled. |
| Prompt synchronous fetch setup failure | NOT RUN | Synchronous `fetch` throws are injected for created and deleted only. |
| No retries in every failure mode | NOT RUN | The tests show immediate created coalescing and later event-driven dispatch after timeout; they do not exhaustively prove absence of retries for every endpoint/status. |
| No unhandled failures | NOT RUN | Node completion after the two injected synchronous throws is useful evidence, but no unhandled-rejection listener/assertion is installed. |
| No plugin-created processes or daemon launch | NOT RUN | The test launches Node as its harness and does not inspect child processes or daemon starts. |
| OpenCode remains usable after failure | NOT RUN | Requires the disposable live runtime procedure; no external model or authentication was invoked here. |

## Created endpoint compliance

The source/template and automated lifecycle test use `POST /sessions` for created delivery, matching design §6. The focused source-derived loopback test was first changed to expect this route and failed while the template still used the obsolete route; it passed after the minimal endpoint correction.

## Runtime record

| Field | Value |
| --- | --- |
| OpenCode version | NOT RUN (reported available: `1.18.29`) |
| Date / operator | NOT RUN |
| Temporary home | NOT RUN |
| Loopback receiver result | NOT RUN |
| User configuration changed | No |
| External model or authentication invoked | No |
