# Zoho Books routing

Route independently by host application, every target application, execution context, capability, requested language, and requested placement. Application ownership and language ownership are orthogonal.

| Request | Skills and output |
|---|---|
| Books as host application | Load `zoho-books`; add the actual language skill only when code uses that language. |
| Books as target application | Load `zoho-books` for Books contracts and retain the host application's skill. |
| Books as a cross-application participant | Load every involved application skill; do not let one application own another's facts. |
| Books Deluge | Load `zoho-books` and `zoho-deluge`; emit `[name].deluge` at the configured placement. |
| External runtime | Load relevant application skills; preserve the requested language and requested placement rather than implying Deluge. |

## Deterministic route

1. Check whether a bounded standard Books capability may satisfy the intent. Present this as advisory, non-blocking guidance and continue an explicit code request.
2. When the code is Deluge, prefer a fully compatible Integration Task before REST v3.
3. A task miss never selects REST automatically. Evaluate REST v3 independently, another verified surface such as an extension or widget, a manual product outcome, or an explicit unsupported outcome.
4. Use one valid path when only one remains. When paths are equally optimal, explain them, recommend one, and wait for selection.

Books-native expenses, items, projects, and time tracking remain Books concerns. Zoho Inventory, Expense, Billing, Payments, Payroll, BillPay, and other adjacent products are outside `zoho-books`; load their own application skills when involved.
