# Native Creator Deluge statements

These Creator-only statements operate on forms in the current application. `zoho-deluge` owns their generic grammar, criteria expressions, collections, assignment syntax, and subform-row construction; this reference owns Creator applicability and effects.

| Operation | Statement | Creator behavior |
|---|---|---|
| `Insert record` | `id = insert into <form_link_name> [<field_link_name>=<expression> ...];` | Target form On Validate and On Success do not execute. |
| `Fetch records` | `<records> = <form_link_name>[<criteria>] [sort by ...] [range from ...];` | Criteria is mandatory; order is not implicit; field and criteria restrictions apply. |
| `Update records` | `<record>.<field_link_name> = <expression>;` | Update a fetched record or collection; restricted field types remain unavailable. |
| `Delete records` | `delete from <form_link_name>[<criteria>];` | Criteria is mandatory; existing-form subform children are delinked unless explicitly deleted. |
| `Insert subform rows` | `<row> = <main_form>.<subform>(); ... input.<subform>.insert(<collection>);` | Custom-sorted subforms are unsupported; failure atomicity depends on the workflow event. |
| `Clear subform rows` | `input.<subform_link_name>.clear();` | Clears every row, not selected rows; existing-form child records remain in the child form. |

Validate the workflow event before use. Insert and clear have different context availability, and native operations must not be generalized to another application.
