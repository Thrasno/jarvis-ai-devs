# Uncertainty and safe errors

Ask only missing facts that change routing or prevent safe generation. Use `verified`, `contradictory`, `unavailable`, and `TBD` exactly; contradictory, unavailable, or TBD operation evidence must fail closed.

Do not invent operations, task names, methods, paths, request or response shapes, identifiers, scopes, permissions, plans, quotas, costs, lock periods, webhook behavior, cross-product ID mappings, or tenant capabilities. Do not conceal known permission failures.

If one verified route remains, explain and use it. If several are equally optimal, recommend one and wait for selection. When evidence is excluded or missing, state the unsupported boundary and offer a safe clarification, runtime validation step, standard product configuration, manual workaround, or another verified surface.
