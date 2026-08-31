# CRM execution contexts

CRM Functions are server-side Deluge. Their exact configured placement and argument mappings are runtime facts. Client Script is browser-side JavaScript and belongs in the requested module, layout, page, and event.

Use a workflow, standalone function, related-list function, schedule, custom button, validation rule, Client Script, REST, or an external runtime only after the context makes that surface safe. Do not generate schedule arguments, implicit custom-button arguments, validation signatures or returns, Quick Create behavior, or Function API endpoint templates without target configuration or newly verified evidence.

Keep Deluge output as `[name].deluge`; keep Client Script output as `[name].js`.
