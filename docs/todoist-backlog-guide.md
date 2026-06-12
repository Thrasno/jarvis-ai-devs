# Todoist Backlog Guide

Use this guide when creating, updating, moving, verifying, or closing backlog items for this repository. The backlog lives in Todoist under project `Jarvis-Dev`, with parent task `JARVIS` as the repo backlog root.

## Quick path

1. Load the Todoist token privately from `~/.config/tokens/todoist.env` into an environment variable. Do not print, echo, copy, or commit the token.
2. Use the Todoist API v1 base URL: `https://api.todoist.com/api/v1`.
3. Find the `Jarvis-Dev` project, then find the `JARVIS` parent task inside it.
4. Create or update tasks with JSON bodies and an `Authorization: Bearer ...` header.
5. Move new backlog items under `JARVIS` with the dedicated move endpoint.
6. Verify the task appears under the expected project and parent before reporting success.

## Safe token handling

| Rule | Why |
|------|-----|
| Read only from `~/.config/tokens/todoist.env`. | Keeps secrets out of the repository. |
| Parse the env file into a process variable. | Allows API calls without exposing the token in docs, logs, or commits. |
| Never run commands that print the token. | Shell transcripts and CI logs can persist secrets. |
| Use `Authorization: Bearer $TODOIST_API_TOKEN`. | Todoist API calls require a bearer token header. |

Treat the env file as secret data. It may contain either a raw token or a `KEY=VALUE` assignment depending on the local machine. Parse it defensively, load only the Todoist token needed for the current process, and avoid debugging with `cat`, `echo`, or verbose command modes that would reveal secret values.

## API basics

| Action | Endpoint |
|--------|----------|
| List projects | `GET /projects` |
| List tasks | `GET /tasks` |
| Create task | `POST /tasks` |
| Update task | `POST /tasks/{task_id}` |
| Move task | `POST /tasks/{task_id}/move` |
| Complete task | `POST /tasks/{task_id}/close` |

All paths are relative to `https://api.todoist.com/api/v1`.

Important gotcha: API v1 list endpoints return a `results` envelope. Read arrays from `.results`, not from the top-level response.

## Practical workflows

### Find the project

Call `GET /projects`, parse `.results[]`, and select the project where `name == "Jarvis-Dev"`. Keep the project ID for later calls.

### Find the parent task

Call `GET /tasks?project_id=<project_id>`, parse `.results[]`, and select the task where `content == "JARVIS"`. Keep the task ID as the backlog parent ID.

### Create a task or subtask

Create tasks with JSON:

```json
{
  "content": "Short actionable task title",
  "description": "Context, acceptance notes, and links.",
  "project_id": "<project_id>"
}
```

If the task must live under `JARVIS`, include `"parent_id": "<jarvis_parent_task_id>"` when creating it. Use the move endpoint only when reorganizing an existing task.

### Update a task

Use `POST /tasks/{task_id}` with a JSON body containing only the fields to change, for example:

```json
{
  "content": "Updated task title",
  "description": "Updated context or acceptance notes."
}
```

Do not use generic task update to set `parent_id`; Todoist API v1 does not accept `parent_id` there.

### Move a task under `JARVIS`

Use the dedicated move endpoint:

```http
POST /tasks/{task_id}/move
Content-Type: application/json
Authorization: Bearer $TODOIST_API_TOKEN

{"parent_id":"<jarvis_parent_task_id>"}
```

### Verify a change

After creating, updating, or moving a task:

1. Re-list tasks for the `Jarvis-Dev` project.
2. Parse `.results[]`.
3. Confirm the task content, description, project ID, and parent relationship are correct.

### Complete or close a task

When the work is finished and accepted, close it with:

```http
POST /tasks/{task_id}/close
Authorization: Bearer $TODOIST_API_TOKEN
```

Verify by listing active tasks again and confirming the closed task no longer appears in the active backlog view.

## Checklist

- [ ] Token path used only as `~/.config/tokens/todoist.env`.
- [ ] No token value printed, copied, committed, or included in output.
- [ ] API base URL is `https://api.todoist.com/api/v1`.
- [ ] List responses are parsed from `.results`.
- [ ] Backlog tasks are in project `Jarvis-Dev` under parent task `JARVIS`.
- [ ] Parent changes use `POST /tasks/{task_id}/move` with `{"parent_id":"..."}`.
