# People entities and identifiers

Runtime metadata and exact operation responses are authoritative. Display names are presentation only unless the operation explicitly accepts them.

| Entity | Parent | Identifier boundary |
|---|---|---|
| Organization | Account | Organization/company ID supplies account context. |
| Form | Account | Keep form label name distinct from form link name (`formLinkName`); selection semantics are operation-specific. |
| View | Form/account | Use the runtime view link name required by the exact view route. |
| Field/component | Form/section | Keep field label name, field link name, display name, and returned field/component ID distinct. Search uses documented link names; form writes use documented labels. |
| Section/tabular section | Form | Use returned section/component ID or link metadata. |
| Form record | Form | Use the People record ID. Responses may expose `Zoho_ID` or `pkId`; updates require `recordid`. Never substitute employee ID without endpoint evidence. |
| Employee | Organization/form | The exact operation decides among record ID, employee ID, email, user ID, or `erecno`. |
| Candidate/onboarding | Organization | Candidate, onboarding, and document IDs are lifecycle-specific. |
| HR case/category | Organization/employee | Use case ID, category ID, and endpoint-specific employee identity. |
| Time client/project/job | Account/client/project | People Time Tracker IDs are not Zoho Projects IDs. |
| Schedule/work item/log/timer/timesheet | Job/employee | Use returned schedule, work-item, log, timer, timesheet, and reference IDs. |
| Leave/grant/comp-off | Employee | Use exact request, grant, comp-off, and leave-type IDs. |
| Leave configuration | Organization | Use holiday, pay-period, and work-calendar IDs. |
| Attendance/shift | Employee/organization | Use entry, shift, schedule, and mapping IDs. |
| Compensation | Employee/organization | Use component, package, currency, revision, salary IDs, and employee `erecno` where documented. |
| LMS hierarchy | Account/course | Keep course, batch, module, entity, content/file/link, session, test, assignment, learner, trainer, room, category, and plan IDs distinct. |
| File/folder | Organization/employee/LMS | General People Files and LMS Files are separate families; use each operation's file, folder/category, path, or resource ID. |
| Variable/group | Organization | Use returned variable key/ID and group ID. |
| Performance | Organization/user | Use competency, KRA, skill, review-question, and user-assignment IDs. |
| Organization structure | Organization | Use the runtime entity/record ID. |
| Survey/report | Survey | Keep survey, recurrence, metric, question, response, and user IDs distinct. |
| HR process | Organization | Use the returned process ID. |

Specialized domains must not route through generic form CRUD without exact evidence. Never assume IDs interoperate across entity or product boundaries.
