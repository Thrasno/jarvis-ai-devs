# Workflows and subforms

## Workflow effects

Native `insert into` targets the current Creator application; the target form's On Validate and On Success scripts do not execute. Do not infer the same behavior for REST or integration tasks.

v2.1 add, update, delete, and upload execute associated workflows by default. Eligible administrators or developers may use operation-specific `skip_workflow` behavior, but it is not universal: add and upload retain blueprints and approvals, while update and delete retain blueprints. v2 pages do not expose `skip_workflow`; do not infer undocumented v2 event behavior or apply v2.1 controls to v2-backed integration tasks.

Confirm the exact event, placement, validation behavior, and intended workflow effects before generating a mutation.

## Subform boundaries

- Build rows using the main form's subform constructor and insert a collection through `input.<subform>.insert(...)` only in supported workflow contexts.
- Insertion is unsupported for custom-sorted subforms. Failure atomicity varies by workflow event.
- `input.<subform_link_name>.clear()` removes every row, not selected rows, and has narrower context availability than insertion.
- Deleting a parent record delinks existing-form child records; it does not delete those child records unless they are explicitly deleted.
- Keep record IDs and subform record IDs distinct. v2.1 representations do not authorize using one as the other.
- Keep the contradictory subform file-download path blocked until official evidence agrees.
