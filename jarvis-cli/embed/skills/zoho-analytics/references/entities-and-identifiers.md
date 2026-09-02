# Analytics entities and identifiers

REST normally requires `orgId` in `ZANALYTICS-ORGID`, `workspaceId`, and operation-specific entity IDs. Organization discovery is the exception: `GET /orgs` cannot require an organization ID that has not yet been discovered. Discover IDs through metadata whenever available; never derive them from display names.

| Entity | Canonical REST identity | Name/discovery surface |
|---|---|---|
| Organization | `orgId` | `GET /orgs` |
| Workspace/database | `workspaceId` | Workspace metadata; Deluge uses `database_name`. |
| View | `viewId` | Get Views or View Details. |
| Table | Table view addressed by `viewId` | Get Table Metadata; Deluge uses table name. |
| Query table | `query-table-id`; creation returns a `viewId` | Query-table metadata. |
| Report/chart/pivot | View ID and type | Get Views or View Details. |
| Dashboard | Dashboard/view identity | Dashboard metadata. |
| Folder | `folderId` | Get Folders. |
| Column | `columnId` where required | Table metadata; row/SQL payloads may use column names. |
| Formula/variable/data source | `formulaId`, `variableId`, `datasourceId` | Corresponding metadata operations. |
| User/group/role | User/email identity, `groupId`, `roleId` | User, group, and role metadata. |
| Slideshow/tag/schedule | `slideId`, `tagId`, `scheduleId` | Corresponding list/detail operations. |
| AutoML | `analysisId`, `modelId`, `deploymentId` | AutoML metadata. |
| Asynchronous job | `jobId` | Import/export status and callback payload. |

Keep REST ID-based identity separate from the legacy name-based Deluge task identity. Resolve names and IDs independently, and state which form the selected surface requires.
