# Zoho Analytics REST v2 catalog

Evidence date: 2026-09-01. The official REST v2 menu and seven official OpenAPI assets contain **176 active verified REST v2 operations**: Data 3, Bulk 12, Modeling 65, Metadata 47, Sharing and collaboration 17, Embed 15, and User management 17. This is a dated catalog boundary, not proof that no separately licensed or undocumented capability exists.

Normal operations use OAuth at `https://{analyticsapi-DC}/restapi/v2` and require `ZANALYTICS-ORGID`; organization discovery is the exception. Paths and scopes below preserve the OpenAPI evidence exactly so contradictions remain visible. Operation IDs are recognition keys, not authorization to skip current official or runtime validation.

## Data — 3 operations

Scopes: `ZohoAnalytics.data.create|update|delete` according to operation.

| Method | OpenAPI path | Operation | Operation ID | OpenAPI scope |
|---|---|---|---|---|
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/rows` | Add Row | `addRow` | `ZohoAnalytics.data.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/rows` | Update Rows | `updateRows` | `ZohoAnalytics.data.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/rows` | Delete Row | `deleteRows` | `ZohoAnalytics.data.delete` |

## Bulk — 12 operations

Imports use `ZohoAnalytics.data.create`; exports use `ZohoAnalytics.data.read`. Preserve synchronous, batch, and asynchronous lifecycles.

| Method | OpenAPI path | Operation | Operation ID | OpenAPI scope |
|---|---|---|---|---|
| `POST` | `/restapi/v2/workspaces/{workspace-id}/data` | Import Data into a New Table (Synchronous) | `importDataNewTable` | `ZohoAnalytics.data.create` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/data` | Import Data into an Existing Table (Synchronous) | `importDataExistingTable` | `ZohoAnalytics.data.create` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/data` | Export Data from a View (Synchronous) | `exportDataView` | `ZohoAnalytics.data.read` |
| `POST` | `/restapi/v2/bulk/workspaces/{workspace-id}/data/batch` | Batch Import Data into New Table | `batchImportNewTable` | `ZohoAnalytics.data.create` |
| `POST` | `/restapi/v2/bulk/workspaces/{workspace-id}/views/{view-id}/data/batch` | Batch Import Data into Existing Table | `batchImportExistingTable` | `ZohoAnalytics.data.create` |
| `GET` | `/restapi/v2/bulk/workspaces/{workspace-id}/data` | Create Export Job using SQL Query (Asynchronous) | `createExportJobSQLQuery` | `ZohoAnalytics.data.read` |
| `POST` | `/restapi/v2/bulk/workspaces/{workspace-id}/data` | Create Import Job for a New Table (Asynchronous) | `createImportJobNewTable` | `ZohoAnalytics.data.create` |
| `GET` | `/restapi/v2/bulk/workspaces/{workspace-id}/views/{view-id}/data` | Create Export Job using View ID (Asynchronous) | `createExportJobViewId` | `ZohoAnalytics.data.read` |
| `POST` | `/restapi/v2/bulk/workspaces/{workspace-id}/views/{view-id}/data` | Create Import Job for an Existing Table (Asynchronous) | `createImportJobExistingTable` | `ZohoAnalytics.data.create` |
| `GET` | `/restapi/v2/bulk/workspaces/{workspace-id}/importjobs/{job-id}` | Get Import Job Details | `getImportJobDetails` | `ZohoAnalytics.data.read` |
| `GET` | `/restapi/v2/bulk/workspaces/{workspace-id}/exportjobs/{job-id}` | Get Export Job Details | `getExportJobDetails` | `ZohoAnalytics.data.read` |
| `GET` | `/restapi/v2/bulk/workspaces/{workspace-id}/exportjobs/{job-id}/data` | Download Exported Data | `downloadExportedData` | `ZohoAnalytics.data.read` |

## Modeling — 65 operations

Scopes follow `ZohoAnalytics.modeling.create|read|update|delete`; operation evidence decides the verb scope, subject to the contradictions below.

| Method | OpenAPI path | Operation | Operation ID | OpenAPI scope |
|---|---|---|---|---|
| `POST` | `/restapi/v2/workspaces` | Create Workspace | `createWorkspace` | `ZohoAnalytics.modeling.create` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}` | Copy Workspace | `copyWorkspace` | `ZohoAnalytics.modeling.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}` | Rename Workspace | `renameWorkspace` | `ZohoAnalytics.modeling.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}` | Delete Workspace | `deleteWorkspace` | `ZohoAnalytics.modeling.delete` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/tables` | Create Table | `createTable` | `ZohoAnalytics.modeling.create` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/querytables` | Create Query Table | `createQueryTable` | `ZohoAnalytics.modeling.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/querytables/{table-id}` | Edit Query Table | `editQueryTable` | `ZohoAnalytics.modeling.update` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/reports` | Create Report | `createReport` | `ZohoAnalytics.modeling.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/reports/{report-id}` | Update Report | `updateReport` | `ZohoAnalytics.modeling.update` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/saveas` | Save As View | `saveAsView` | `ZohoAnalytics.modeling.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/movetofolder` | Move Views To Folder | `moveViewsToFolder` | `ZohoAnalytics.modeling.update` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}` | Rename View | `renameView` | `ZohoAnalytics.modeling.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}` | Delete View | `deleteView` | `ZohoAnalytics.modeling.delete` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/trash/{view-id}` | Restore Trash View | `restoreTrashView` | `ZohoAnalytics.modeling.create` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/trash/{view-id}` | Delete Trash View | `deleteTrashView` | `ZohoAnalytics.modeling.delete` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/folders` | Create Folder | `createFolder` | `ZohoAnalytics.modeling.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/folders/{folder-id}` | Rename Folder | `renameFolder` | `ZohoAnalytics.modeling.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/folders/{folder-id}` | Delete Folder | `deleteFolder` | `ZohoAnalytics.modeling.delete` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/folders/{folder-id}/default` | Make Default Folder | `makeDefaultFolder` | `ZohoAnalytics.modeling.update` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/folders/{folder-id}/move` | Change Folder Hierarchy | `changeFolderHierarchy` | `ZohoAnalytics.modeling.update` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/folders/{folder-id}/reorder` | Change Folder Position | `changeFolderPosition` | `ZohoAnalytics.modeling.update` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/data/sort` | Sort Data by Columns | `sortDataByColumns` | `ZohoAnalytics.modeling.update` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/columns` | Add Column | `addColumn` | `ZohoAnalytics.modeling.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/columns/{column-id}` | Rename Column | `renameColumn` | `ZohoAnalytics.modeling.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/columns/{column-id}` | Delete Column | `deleteColumn` | `ZohoAnalytics.modeling.delete` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/columns/{column-id}/lookup` | Add Lookup | `addLookup` | `ZohoAnalytics.modeling.create` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/columns/{column-id}/lookup` | Remove Lookup | `removeLookup` | `ZohoAnalytics.modeling.delete` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/columns/hide` | Hide Columns | `hideColumns` | `ZohoAnalytics.modeling.update` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/columns/show` | Show Columns | `showColumns` | `ZohoAnalytics.modeling.update` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/columns/reorder` | Reorder Columns | `reorderColumns` | `ZohoAnalytics.modeling.update` |
| `POST` | `/restapi/v2/workspaces/{source-workspace-id}/views/copy` | Copy Views | `copyViews` | `ZohoAnalytics.modeling.create` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/formulas/copy` | Copy Formulas | `copyFormulas` | `ZohoAnalytics.modeling.create` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/similarviews` | Create Similar Views | `createSimilarViews` | `ZohoAnalytics.modeling.create` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/autoanalyse` | Auto Analyse View | `autoAnalyseView` | `ZohoAnalytics.modeling.create` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/columns/{column-id}/autoanalyse` | Auto Analyse Column | `autoAnalyseColumn` | `ZohoAnalytics.modeling.create` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/variables` | Get all variables in a workspace | `getVariables` | `ZohoAnalytics.modeling.read` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/variables` | Create a variable | `createVariable` | `ZohoAnalytics.modeling.create` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/variables/{variable-id}` | Get details of a specific variable | `getVariableDetails` | `ZohoAnalytics.modeling.read` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/variables/{variable-id}` | Update a variable | `updateVariable` | `ZohoAnalytics.modeling.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/variables/{variable-id}` | Delete a variable | `deleteVariable` | `ZohoAnalytics.modeling.delete` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/customformulas` | Add Formula Column | `addFormulaColumn` | `ZohoAnalytics.modeling.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/customformulas/{formula-id}` | Edit Formula Column | `editFormulaColumn` | `ZohoAnalytics.modeling.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/customformulas/{formula-id}` | Delete Formula Column | `deleteFormulaColumn` | `ZohoAnalytics.modeling.delete` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/aggregateformulas` | Add Aggregate Formula | `addAggregateFormula` | `ZohoAnalytics.modeling.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/aggregateformulas/{formula-id}` | Edit Aggregate Formula | `editAggregateFormula` | `ZohoAnalytics.modeling.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/aggregateformulas/{formula-id}` | Delete Aggregate Formula | `deleteAggregateFormula` | `ZohoAnalytics.modeling.delete` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/emailschedules` | Create Email Schedule | `createEmailSchedule` | `ZohoAnalytics.modeling.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/emailschedules/{schedule-id}` | Update Email Schedule | `updateEmailSchedule` | `ZohoAnalytics.modeling.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/emailschedules/{schedule-id}` | Delete Email Schedule | `deleteEmailSchedule` | `ZohoAnalytics.modeling.delete` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/emailschedules/{schedule-id}/trigger` | Trigger Email Schedule | `triggerEmailSchedule` | `ZohoAnalytics.modeling.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/emailschedules/{schedule-id}/status` | Change Email Schedule Status | `changeEmailScheduleStatus` | `ZohoAnalytics.modeling.update` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/tags` | Create Tag | `createTag` | `ZohoAnalytics.modeling.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/tags/{tag-id}` | Update Tag | `updateTag` | `ZohoAnalytics.modeling.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/tags/{tag-id}` | Delete Tag | `deleteTag` | `ZohoAnalytics.modeling.delete` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/tags/{tag-id}/views` | Add Tag To Multiple Views | `addTagToViews` | `ZohoAnalytics.modeling.create` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/tags/{tag-id}/views` | Remove Tag From Multiple Views | `removeTagFromViews` | `ZohoAnalytics.modeling.delete` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/tags` | Add Multiple Tags To View | `addTagsToView` | `ZohoAnalytics.modeling.create` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/tags` | Remove Multiple Tags From View | `removeTagsFromView` | `ZohoAnalytics.modeling.delete` |
| `POST` | `/restapi/v2/automl/workspaces/{workspace-id}/analysis` | Create AutoML Analysis | `createAutoMLAnalysis` | `ZohoAnalytics.modeling.create` |
| `POST` | `/restapi/v2/automl/workspaces/{workspace-id}/analysis/{analysis-id}/models/{model-id}/deployments` | Create AutoML Analysis Deployment | `createAutoMLAnalysisDeployment` | `ZohoAnalytics.modeling.create` |
| `POST` | `/restapi/v2/automl/workspaces/{workspace-id}/analysis/{analysis-id}/models/{model-id}/whatif` | AutoML What If Analysis | `autoMLWhatIfAnalysis` | `ZohoAnalytics.modeling.create` |
| `POST` | `/restapi/v2/automl/workspaces/{workspace-id}/analysis/{analysis-id}/deployments/{deployment-id}/execute` | Run AutoML Analysis | `runAutoMLAnalysis` | `ZohoAnalytics.modeling.create` |
| `DELETE` | `/restapi/v2/automl/workspaces/{workspace-id}/analysis/{analysis-id}/deployments/{deployment-id}` | Delete AutoML Analysis Model Deployment | `deleteAutoMLAnalysisModelDeployment` | `ZohoAnalytics.modeling.delete` |
| `DELETE` | `/restapi/v2/automl/workspaces/{workspace-id}/analysis/{analysis-id}/models/{model-id}` | Delete AutoML Analysis Model | `deleteAutoMLAnalysisModel` | `ZohoAnalytics.modeling.delete` |
| `DELETE` | `/restapi/v2/automl/workspaces/{workspace-id}/analysis/{analysis-id}` | Delete AutoML Analysis | `deleteAutoMLAnalysis` | `ZohoAnalytics.modeling.delete` |

## Metadata — 47 operations

Reads primarily use `ZohoAnalytics.metadata.read`; mutations use `metadata.create|update`, subject to the incomplete prerequisite evidence below.

| Method | OpenAPI path | Operation | Operation ID | OpenAPI scope |
|---|---|---|---|---|
| `GET` | `/restapi/v2/orgs` | Get organizations | `getOrganizations` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces` | Get all workspaces | `getAllWorkspaces` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/owned` | Get owned workspaces | `getOwnedWorkspaces` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/shared` | Get shared workspaces | `getSharedWorkspaces` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views` | Get Views | `getViews` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/folders` | Get Folders | `getFolders` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/recentviews` | Get Recent Views | `getRecentViews` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/trash` | Get Trash Views | `getTrashViews` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/dashboards` | Get Dashboards | `getDashboards` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/dashboards/owned` | Get Owned Dashboards | `getOwnedDashboards` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/dashboards/shared` | Get Shared Dashboards | `getSharedDashboards` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/secretkey` | Get Workspace Secret Key | `getWorkspaceSecretKey` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}` | Get Workspace Details | `getWorkspaceDetails` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/views/{view-id}` | Get View Details | `getViewDetails` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/querytables` | Get Query Tables | `getQueryTables` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/querytables/{query-table-id}` | Get QueryTable Details | `getQueryTableDetails` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/tags` | Get Tags List | `getTagsList` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/tags/{tag-id}/views` | Get Tagged Views | `getTaggedViews` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/tags` | Get View Tags | `getViewTags` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/metadata` | Get Table Metadata | `getTableMetadata` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/dependents` | Get View Dependents | `getViewDependents` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/columns/{column-id}/dependents` | Get Column Dependents | `getColumnDependents` | `ZohoAnalytics.metadata.read` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/favorite` | Add Favorite Workspace | `addFavoriteWorkspace` | `ZohoAnalytics.metadata.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/favorite` | Remove Favorite Workspace | `removeFavoriteWorkspace` | `ZohoAnalytics.metadata.update` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/default` | Add Default Workspace | `addDefaultWorkspace` | `ZohoAnalytics.metadata.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/default` | Remove Default Workspace | `removeDefaultWorkspace` | `ZohoAnalytics.metadata.update` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/favorite` | Add Favorite View | `addFavoriteView` | `ZohoAnalytics.metadata.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/favorite` | Remove Favorite View | `removeFavoriteView` | `ZohoAnalytics.metadata.update` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/template/data` | Export as Template | `exportAsTemplate` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/customformulas` | Get Custom Formula List | `getCustomFormulaList` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/aggregateformulas` | Get Aggregate Formula List | `getAggregateFormulaList` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/aggregateformulas` | Get Aggregate Formulas In Workspace | `getAggregateFormulasInWorkspace` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/aggregateformulas/{formula-id}/dependents` | Get Aggregate Formula Dependents | `getAggregateFormulaDependents` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/aggregateformulas/{formula-id}/value` | Get Aggregate Formula Value | `getAggregateFormulaValue` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/metadetails` | Get Meta Details | `getMetaDetails` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/emailschedules` | Get Email Schedules | `getEmailSchedules` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/datasources` | Get Datasources | `getDatasources` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/importdetails` | Get Last Import Details | `getLastImportDetails` | `ZohoAnalytics.metadata.read` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/datasource/{datasource-id}/sync` | Sync Data | `syncDatasource` | `ZohoAnalytics.metadata.create` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/sync` | Refetch Data | `refetchDatasource` | `ZohoAnalytics.metadata.create` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/datasource/{datasource-id}` | Update Datasource Connection | `updateDatasourceConnection` | `ZohoAnalytics.metadata.update` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/wlaccess` | Enable Domain Workspace | `enableDomainWorkspace` | `ZohoAnalytics.metadata.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/wlaccess` | Disable Domain Workspace | `disableDomainWorkspace` | `ZohoAnalytics.metadata.update` |
| `GET` | `/restapi/v2/automl/analysis` | Get AutoML Analysis In Org | `getAutoMLAnalysisInOrg` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/automl/workspaces/{workspace-id}/analysis` | Get AutoML Analysis In Workspace | `getAutoMLAnalysisInWorkspace` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/automl/workspaces/{workspace-id}/analysis/{analysis-id}` | Get AutoML Analysis Details | `getAutoMLAnalysisDetails` | `ZohoAnalytics.metadata.read` |
| `GET` | `/restapi/v2/automl/workspaces/{workspace-id}/analysis/{analysis-id}/models/{model-id}/deployments` | Get Deployments For A Model | `getDeploymentsForModel` | `ZohoAnalytics.metadata.read` |

## Sharing and collaboration — 17 operations

Scopes are `ZohoAnalytics.share.read|create|update|delete` according to operation.

| Method | OpenAPI path | Operation | Operation ID | OpenAPI scope |
|---|---|---|---|---|
| `GET` | `/restapi/v2/orgadmins` | Get Org Admins | `getOrgAdmins` | `ZohoAnalytics.share.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/admins` | Get Workspace Admins | `getWorkspaceAdmins` | `ZohoAnalytics.share.read` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/admins` | Add Workspace Admins | `addWorkspaceAdmins` | `ZohoAnalytics.share.create` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/admins` | Remove Workspace Admins | `removeWorkspaceAdmins` | `ZohoAnalytics.share.delete` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/share` | Get Workspace Shared Details | `getWorkspaceSharedDetails` | `ZohoAnalytics.share.read` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/share` | Share Views | `shareViews` | `ZohoAnalytics.share.create` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/views/share` | Remove Share | `removeShare` | `ZohoAnalytics.share.delete` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/share` | Update Shared Details For View | `UpdateSharedDetailsForView` | `ZohoAnalytics.share.update` |
| `GET` | `/workspaces/{workspace-id}/views/{view-id}/share/mypermissions` | Get My Permissions | `getMyPermissions` | `ZohoAnalytics.share.read` |
| `GET` | `/workspaces/{workspace-id}/share/shareddetails` | Get Shared Details For Views | `getSharedDetailsForViews` | `ZohoAnalytics.share.read` |
| `GET` | `/workspaces/{workspace-id}/groups` | Get Groups | `getGroups` | `ZohoAnalytics.share.read` |
| `POST` | `/workspaces/{workspace-id}/groups` | Create Group | `createGroup` | `ZohoAnalytics.share.create` |
| `GET` | `/workspaces/{workspace-id}/groups/{group-id}` | Get Group Details | `getGroupDetails` | `ZohoAnalytics.share.read` |
| `PUT` | `/workspaces/{workspace-id}/groups/{group-id}` | Rename Group | `renameGroup` | `ZohoAnalytics.share.update` |
| `DELETE` | `/workspaces/{workspace-id}/groups/{group-id}` | Delete Group | `deleteGroup` | `ZohoAnalytics.share.delete` |
| `POST` | `/workspaces/{workspace-id}/groups/{group-id}/members` | Add Group Members | `addGroupMembers` | `ZohoAnalytics.share.create` |
| `DELETE` | `/workspaces/{workspace-id}/groups/{group-id}/members` | Remove Group Members | `removeGroupMembers` | `ZohoAnalytics.share.delete` |

## Embed — 15 operations

Scopes are `ZohoAnalytics.embed.read|create|update|delete`, subject to incomplete mutation-scope prerequisite evidence.

| Method | OpenAPI path | Operation | Operation ID | OpenAPI scope |
|---|---|---|---|---|
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/publish` | Get View URL | `getViewUrl` | `ZohoAnalytics.embed.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/publish/embed` | Get Embed URL | `getEmbedUrl` | `ZohoAnalytics.embed.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/publish/privatelink` | Get Private URL | `getPrivateUrl` | `ZohoAnalytics.embed.read` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/publish/privatelink` | Create Private URL | `createPrivateUrl` | `ZohoAnalytics.embed.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/publish/privatelink` | Remove Private Access | `removePrivateAccess` | `ZohoAnalytics.embed.delete` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/publish/public` | Make Views Public | `makeViewsPublic` | `ZohoAnalytics.embed.create` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/publish/public` | Remove Public Permission | `removePublicPermission` | `ZohoAnalytics.embed.delete` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/publish/config` | Get Publish Configurations | `getPublishConfigurations` | `ZohoAnalytics.embed.read` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/views/{view-id}/publish/config` | Update Publish Configurations | `updatePublishConfigurations` | `ZohoAnalytics.embed.update` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/slides` | Get Slideshows | `getSlideshows` | `ZohoAnalytics.embed.read` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/slides` | Create Slideshow | `createSlideshow` | `ZohoAnalytics.embed.create` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/slides/{slide-id}` | Get Slideshow Details | `getSlideshowDetails` | `ZohoAnalytics.embed.read` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/slides/{slide-id}` | Update Slideshow | `updateSlideshow` | `ZohoAnalytics.embed.update` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/slides/{slide-id}` | Delete Slideshow | `deleteSlideshow` | `ZohoAnalytics.embed.delete` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/slides/{slide-id}/publish` | Get Slideshow URL | `getSlideshowUrl` | `ZohoAnalytics.embed.read` |

## User management — 17 operations

Scopes are `ZohoAnalytics.usermanagement.read|create|update|delete` according to operation; retain the resource-details path contradiction below.

| Method | OpenAPI path | Operation | Operation ID | OpenAPI scope |
|---|---|---|---|---|
| `GET` | `/restapi/v2/users` | Get Users | `getUsers` | `ZohoAnalytics.usermanagement.read` |
| `POST` | `/restapi/v2/users` | Add Users | `addUsers` | `ZohoAnalytics.usermanagement.create` |
| `DELETE` | `/restapi/v2/users` | Remove Users | `removeUsers` | `ZohoAnalytics.usermanagement.delete` |
| `PUT` | `/restapi/v2/users/active` | Activate Users | `activateUsers` | `ZohoAnalytics.usermanagement.update` |
| `PUT` | `/restapi/v2/users/inactive` | Deactivate Users | `deActivateUsers` | `ZohoAnalytics.usermanagement.update` |
| `PUT` | `/restapi/v2/users/role` | Change User Role | `changeUserRole` | `ZohoAnalytics.usermanagement.update` |
| `GET` | `/restapi/v2/subscription` | Get Subscription Details | `getSubscriptionDetails` | `ZohoAnalytics.usermanagement.read` |
| `GET` | `/restapi/v2/resources` | Get Resource Details | `getResourceDetails` | `ZohoAnalytics.usermanagement.read` |
| `GET` | `/restapi/v2/workspaces/{workspace-id}/users` | Get Workspace Users | `getWorkspaceUsers` | `ZohoAnalytics.usermanagement.read` |
| `POST` | `/restapi/v2/workspaces/{workspace-id}/users` | Add Workspace Users | `addWorkspaceUsers` | `ZohoAnalytics.usermanagement.create` |
| `DELETE` | `/restapi/v2/workspaces/{workspace-id}/users` | Delete Workspace Users | `deleteWorkspaceUsers` | `ZohoAnalytics.usermanagement.delete` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/users/status` | Change Workspace Users Status | `changeWorkspaceUsersStatus` | `ZohoAnalytics.usermanagement.update` |
| `PUT` | `/restapi/v2/workspaces/{workspace-id}/users/role` | Change Workspace Users Role | `changeWorkspaceUsersRole` | `ZohoAnalytics.usermanagement.update` |
| `GET` | `/restapi/v2/orgs/roles` | Get Custom Roles | `getCustomRoles` | `ZohoAnalytics.usermanagement.read` |
| `POST` | `/restapi/v2/orgs/roles` | Create Custom Role | `createCustomRole` | `ZohoAnalytics.usermanagement.create` |
| `PUT` | `/restapi/v2/orgs/roles/{role-id}` | Update Custom Role | `updateCustomRole` | `ZohoAnalytics.usermanagement.update` |
| `DELETE` | `/restapi/v2/orgs/roles/{role-id}` | Delete Custom Role | `deleteCustomRole` | `ZohoAnalytics.usermanagement.delete` |

## Contradictions and fail-closed gates

These conflicts are part of the catalog and fail closed; do not silently choose a path or scope:

- The import-job scope conflicts: the operation page and OpenAPI disagree between `ZohoAnalytics.data.create` and `ZohoAnalytics.data.read`.
- The Modeling prerequisite table omits `modeling.read`. The Save As scope conflicts: the operation page says `modeling.update`, while OpenAPI says `modeling.create`.
- Metadata prerequisites document only `metadata.read|all`, while operation pages/OpenAPI use create/update mutation scopes; one operation page misspells `metadata.updatde`.
- OpenAPI omits `/restapi/v2` from nine group/permission paths while the operation pages include it.
- Embed prerequisites list only `embed.read|all`, while operation pages/OpenAPI use create/update/delete mutation scopes.
- Resource Details conflicts between operation-page `/resourceDetails` and OpenAPI `/resources`.

Unknown operation costs and per-operation plan gates remain unresolved. Warn and require runtime validation; never infer them from another operation.
