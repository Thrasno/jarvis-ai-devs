# Hook End Evidence Compatibility

`RunSessionStop` now derives `directory` from `payload.directory` or `payload.cwd` and derives the canonical `project` with `project.DetectProject(directory)`. It forwards both values, with the resolved session ID, to `PostSessionEnd`.

The hook end request remains `POST /sessions/{id}/end`, but `{id}` is URL path-escaped. Its exact JSON body is:

```json
{"summary":"","project":"<canonical>","directory":"<directory>","client":"hook"}
```

`PostSessionEnd` consequently changed from a session-ID-only internal caller to require project and directory evidence. Existing callers must provide that evidence; the native Stop hook is updated here. Hook behavior remains fail-open: caller context is preserved, a 404 is non-fatal, and transport failures still produce an empty hook response.

Legacy receivers that only relied on the empty `summary` remain compatible with that field. Receivers supporting atomic end materialization can use the added evidence and `hook` client attribution; this slice adds no daemon transport behavior.
