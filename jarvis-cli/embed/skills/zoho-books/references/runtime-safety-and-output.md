# Runtime safety, uncertainty, and output

## Runtime authority and prerequisites

Runtime organization metadata, settings, plan, region, permissions, available providers, and configured connections override static recognition. Check plan only for capability availability. Ask only for missing organization IDs, record IDs, operation-specific API fields, scopes, region/domain, provider, settings, or placement facts that change routing or prevent safe generation.

Use named OAuth connections and exact operation scopes. The Books default is `conpas_books`. Never request, embed, or expose secrets, including access or refresh tokens, client secrets, API keys, passwords, and credential-bearing URLs. Configure and authorize connections through secure deployment configuration.

## Generation versus execution

An explicit request for destructive or financial code authorizes generation without redundant confirmation. Live execution is excluded from this skill. Any future live destructive or financially sensitive action must provide a concrete impact warning and obtain explicit confirmation immediately before execution.

## Unknowns and capability boundaries

`get_delivery_challan_attachment` publishes no response schema; its payload remains `TBD` and the output must not fabricate a binary, string, or wrapper shape. Keep every other unevidenced task, endpoint, response, field, scope, ID, placement, plan, provider, or tenant capability explicit rather than guessing.

Adjacent products require their own application contracts. A task or REST miss may end in another verified surface, a manual outcome, or an explicit unsupported result. Offer a safe clarification or verified alternative. Catalog updates are manual and require an explicit maintainer decision; never imply automatic freshness.

Do not state volatile numeric quotas, credits, concurrency thresholds, capacity estimates, or timeouts. Discuss those only as qualitative risks and consult runtime/administrator evidence when they decide safety.

## Generated output checklist

Match the selected language and surface. Include:

- selected surface and exact placement/configuration;
- argument mapping, IDs, API field names, and per-operation prerequisites;
- named connection and operation-appropriate OAuth scope family;
- assumptions, unresolved runtime facts, and capability boundaries;
- the standard-capability advisory without blocking requested code;
- test cases with expected outcomes, including relevant error and destructive cases.

Executable Deluge validation is required only when a verifiable runtime is available. Otherwise provide deterministic cases and clearly label runtime validation as pending.
