# Current Projects REST API

Direct REST starts at `https://projectsapi.zoho.com` and uses the exact operation section's `/api/v3` or `/api/v3.1` path. Never rewrite `/restapi`, `/api/v3`, or `/api/v3.1` by string substitution. URL grammar, method, hierarchy, scope, pagination, and response handling are operation-specific.

The official snapshot dated 2026-09-01 contains 29 families, 489 sections, and 478 unique operation identities. Of those sections, 467 use v3 and 22 use v3.1. The 11 duplicated operation identities remain `contradictory`; do not silently prefer a method, path, version, or scope.

The [operation-level catalog](current-rest-operations.csv) records every official section. Its repeated rows preserve both variants of each contradictory identity, including exact method, path, version, and scope. `TBD` means the official section or approved contract does not resolve that value; it is not permission to infer one.

| Family | Sections | v3 | v3.1 | Method/context and scope family | Evidence |
|---|---:|---:|---:|---|---|
| Portals | 10 | 10 | 0 | GET, POST, DELETE; account or portal; `portals.*` and some Zoho Files reads | verified |
| Module Meta | 32 | 32 | 0 | GET, POST, PATCH, PUT, DELETE; portal through option; `custom_fields.*` | verified |
| Users | 40 | 34 | 6 | GET, POST, PATCH, DELETE; portal/project/user; `users.*` | contradictory variants |
| Projects | 27 | 27 | 0 | GET, POST, PATCH, DELETE; portal then project; `projects.*`, `portals.*`, `projectgroups.*` | verified; one scope TBD |
| Phases | 17 | 17 | 0 | GET, POST, PATCH, DELETE; project/phase; `milestones.*` | verified |
| Issues | 56 | 40 | 16 | GET, POST, PATCH, PUT, DELETE; nested issue resources; `bugs.*`, some `tasks.READ` | contradictory variants |
| Task Lists | 19 | 19 | 0 | GET, POST, PATCH, DELETE; project/list; `tasklists.*` | verified |
| Tasks | 47 | 47 | 0 | GET, POST, PATCH, DELETE; project/task; `tasks.*` | verified |
| Time Logs | 30 | 30 | 0 | GET, POST, PATCH, DELETE; project/log and optional task/issue; `timesheets.*` | verified |
| Custom Module Records | 33 | 33 | 0 | GET, POST, PATCH, DELETE; portal/entity/record; `portals.*`, `custom_fields.*` | verified |
| Forums | 21 | 21 | 0 | GET, POST, PATCH, DELETE; project/forum/comment; `forums.*` | verified |
| Events | 11 | 11 | 0 | GET, POST, PATCH, DELETE; project calendar events/comments; `events.*` | verified; not webhooks |
| Teams | 9 | 9 | 0 | GET, POST, PATCH, DELETE; portal/team/project; `teams.*` | verified |
| Profiles | 6 | 6 | 0 | GET, POST, PATCH, DELETE; portal/profile; `portals.*` | verified |
| Roles | 6 | 6 | 0 | GET, POST, PATCH, DELETE; portal/role; `portals.*` | verified |
| Permissions | 17 | 17 | 0 | GET, PATCH; profile or project/user; `portals.READ/UPDATE` | verified |
| Clients And Customers | 20 | 20 | 0 | GET, POST, PATCH, DELETE; portal/customer/project; `clients.*` | verified |
| Contacts | 10 | 10 | 0 | GET, POST, PATCH, DELETE; portal/customer/contact/project; `users.*` | verified |
| Tags | 7 | 7 | 0 | GET, POST, PATCH, DELETE; portal/tag/project/entity; `tags.*` | verified |
| Documents | 12 | 12 | 0 | GET, POST, PUT, PATCH, DELETE; project/team folder/folder; `documents.*` plus WorkDrive | verified; cross-product |
| Attachments | 6 | 6 | 0 | GET, POST, DELETE; portal/project/entity; `portals.*` plus WorkDrive | verified; cross-product |
| Reports | 3 | 3 | 0 | GET; portal/project; `portals.READ` | verified |
| Integrations | 1 | 1 | 0 | GET; portal/module; `integrations.READ` | verified |
| Leaves | 5 | 5 | 0 | GET, POST, PATCH, DELETE; portal/user/leave; `leave.*` | verified |
| Search | 2 | 2 | 0 | GET; portal and optional project; `portals.READ`, secure search | verified |
| Feed | 2 | 2 | 0 | GET, POST; portal/project; `status.READ/CREATE` | verified |
| Setup | 33 | 33 | 0 | GET, POST, PATCH, PUT, DELETE; portal configuration; `portals.*`, `extensions.*` | verified |
| Automation | 5 | 5 | 0 | GET, POST; blueprint/transition/task or function; tasks and function scopes | verified but incomplete |
| Bulk Read | 2 | 2 | 0 | GET, POST; portal/module/job; `bulk.READ` | verified |

Before generating a call, select one exact catalog row containing operation name, method, exact path, version, hierarchy IDs, scope or permission, plan state, lifecycle class, official fragment URL, snapshot date `2026-09-01`, and evidence state. A family row never authorizes an endpoint. Only a `verified` operation row can authorize generation; a `contradictory` row requires approved version policy, and any `TBD` field requires runtime validation.

Duplicated user-detail/update, project-user, issue-transition, issue-linking, and issue-resolution identities remain contradictory. Request the approved version policy instead of guessing.

Honor documented pagination for the exact operation. Current REST throttling is 200 requests per endpoint in two minutes, independent per endpoint. Exceeding it blocks that endpoint for ten minutes; inspect rate-limit and `Retry-After` headers. Bound page traversal, avoid redundant calls, and do not invent cursors, page sizes, or retry timing.

Official `/restapi` pages and Deluge examples are legacy evidence. A returned legacy link does not authorize a new direct legacy call. Warn when existing legacy code is read or changed, obtain explicit consent before migration, and migrate only when an exact verified modern equivalent exists.
