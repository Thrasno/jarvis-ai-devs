# Lifecycle operations and webhooks

Treat non-CRUD operations as explicit lifecycle actions. Preserve the exact official operation and state transition, including publish, unpublish, approve, cancel, pause, resume, enroll, unenroll, enable, disable, mark, and reminder actions. Opposing directions such as pause/resume, enable/disable, publish/unpublish, and enroll/unenroll must remain distinct in lifecycle metadata. Do not collapse them into generic update or form CRUD. Other verbs such as trigger, reopen, acknowledge, share, start, stop, or complete remain operation-specific actions.

Neither active REST index exposes webhook-management or event-subscription endpoints. Official People product help documents webhooks as product configuration, not verified REST CRUD. Do not generate a webhook management endpoint.

Webhook trigger coverage, payload contract, retries, signing, and plan availability remain `TBD`. Describe only verified product/manual configuration, require runtime validation, and fail closed rather than inventing API behavior.
