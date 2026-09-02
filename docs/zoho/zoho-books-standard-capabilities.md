# Zoho Books standard-capability taxonomy

> **Readiness: READY as a bounded intent taxonomy with 20 capability families.** This is not an exhaustive UI mirror. It helps route a request toward standard Books behavior before Integration Tasks or REST, while plan, country edition, region, permissions, payment/bank provider, and the live organization remain authoritative.

## How to use this taxonomy

| Status | Meaning |
|---|---|
| `full` | Official Books functionality can satisfy the stable intent without custom code when runtime availability and configuration fit. |
| `partial` | Books covers a material portion, but the requested edge, orchestration, or external behavior may still require another surface. |
| `hybrid` | Books provides the trigger/container/UI, but the intended solution invokes custom code, a webhook, an extension, a widget, or another system. |
| `none_verified` | The inspected official sources do not verify a standard Books replacement. This is not proof that the product can never do it. |
| `TBD` | Evidence is inaccessible, conflicting, dynamic, or too tenant/region-specific to classify safely. |

The advice sequence is: standard Books first, compatible Integration Task second, REST v3 third, then another verified surface/manual outcome. Standard advice is non-blocking when the user explicitly requested code.

## Verified capability families

| # | Stable intent pattern | Status | Standard Books evidence and routing boundary | Official source(s), accessed 2026-08-31 |
|---:|---|---|---|---|
| 1 | Transaction lifecycles | `full` | Books provides native sales and purchase transaction modules and documented lifecycle actions. Use native configuration/UI when it fully meets the intent; lifecycle availability is transaction-, plan-, and organization-specific. | [Invoices](https://www.zoho.com/books/help/invoice/), [quotes](https://www.zoho.com/books/help/quote/), [sales orders](https://www.zoho.com/books/help/sales-order/), [bills](https://www.zoho.com/books/help/bills/), [purchase orders](https://www.zoho.com/books/help/purchase-order/) |
| 2 | Recurring invoices | `full` | Native recurring profiles can create/send recurring invoices and manage their workflow. Do not replace a configured recurring profile with a custom scheduler unless the requested behavior exceeds it. | [Recurring invoices](https://www.zoho.com/books/help/recurring-invoice/) |
| 3 | Reminders | `full` | Native reminder settings and invoice reminder behavior cover standard payment follow-up. Custom code is justified only for requirements outside configured reminder channels/criteria. | [Reminders](https://www.zoho.com/books/help/settings/reminders.html) |
| 4 | Transaction approvals | `full` | Books documents simple, multi-level, and custom transaction approvals. Prefer approval configuration for controlled submit/approve/reject lifecycles. | [Transaction approval](https://www.zoho.com/books/help/transaction-approval/), [configure approvals](https://www.zoho.com/books/help/transaction-approval/configure-approvals.html) |
| 5 | Workflow rules and actions | `hybrid` | Workflow rules natively provide event/date triggers and criteria. Email alerts, in-app notifications, and field updates can be declarative; webhooks and functions intentionally cross into external/custom code. | [Workflow rules](https://www.zoho.com/books/help/settings/automation/workflow-rules.html), [functions](https://www.zoho.com/books/help/settings/automation/workflow-actions/functions.html), [webhooks](https://www.zoho.com/books/help/settings/automation/workflow-actions/webhooks.html) |
| 6 | Fields and validation | `full` | Fields support typed organization data, mandatory/default/unique/input-format behavior, API field names, privacy, and role access; validation rules provide declarative record validation where supported. Runtime module/plan settings decide availability. | [Fields](https://www.zoho.com/books/help/settings/customization/custom-fields.html), [validation rules](https://www.zoho.com/books/help/settings/customization/validation-rules.html) |
| 7 | Views, locking, and buttons | `hybrid` | Custom views and record locking are native controls. Custom buttons provide a native UI entry point but may invoke custom behavior, so button-backed solutions are hybrid rather than purely declarative. | [Custom views](https://www.zoho.com/books/help/settings/customization/custom-views.html), [record locking](https://www.zoho.com/books/help/settings/customization/record-locking.html), [custom buttons](https://www.zoho.com/books/help/settings/customization/custom-buttons.html) |
| 8 | Custom modules | `partial` | Books supports custom modules, preferences, blueprints, layout rules, and portal exposure. They can replace external record stores for supported organization data, but they are not proof that every standard-module lifecycle or API behavior generalizes. | [Custom modules](https://www.zoho.com/books/help/custom-modules/), [blueprints](https://www.zoho.com/books/help/custom-modules/blueprints.html), [layout rules](https://www.zoho.com/books/help/custom-modules/layout-rules.html) |
| 9 | PDF and email templates | `partial` | Books provides PDF/email template configuration. Use it for supported document presentation; custom HTML/CSS or external rendering remains a separate implementation decision. | [PDF templates](https://www.zoho.com/books/help/settings/templates.html), [emails](https://www.zoho.com/books/help/settings/emails.html) |
| 10 | Documents | `full` | The Documents area provides Books-native document management. A requirement for files tied to a specific API resource still needs exact operation evidence. | [Documents](https://www.zoho.com/books/help/documents/documents.html) |
| 11 | Banking and reconciliation | `full` | Books documents bank accounts/feeds, matching and categorization, transaction rules, and reconciliation. Bank/feed availability is provider- and region-dependent at runtime. | [Banking](https://www.zoho.com/books/help/banking/), [transaction rules](https://www.zoho.com/books/help/banking/transaction-rule.html), [reconciliation](https://www.zoho.com/books/help/banking/reconciliation.html) |
| 12 | Taxes and compliance | `partial` | Books provides tax settings, tax automation, reports, and region-specific e-invoicing/compliance pages. Never generalize one country edition's rules to another; live edition/settings are authoritative. | [Taxes](https://www.zoho.com/books/help/settings/taxes.html), [advanced tax automation](https://www.zoho.com/books/help/settings/tax-automation.html), [e-invoicing](https://www.zoho.com/books/help/e-invoicing/spain.html) |
| 13 | Currencies | `full` | Native currency settings and base-currency adjustments cover supported multi-currency accounting intents. Exchange-rate source and availability remain organization-specific. | [Currencies](https://www.zoho.com/books/help/settings/currencies.html), [base currency adjustment](https://www.zoho.com/books/help/accountant/base-currency.html) |
| 14 | Customer and vendor portals | `full` | Books provides customer and vendor portals, preferences, MFA, and documented custom-module exposure. Exact actions remain portal- and organization-configured. | [Customer portal](https://www.zoho.com/books/help/customer-portal/), [vendor portal](https://www.zoho.com/books/help/vendor-portal/) |
| 15 | Reports | `full` | Built-in and custom reports cover accounting, sales, inventory, receivables, payables, tax, and activity analysis. Use REST only when the required extraction/automation is not met by report configuration/export. | [Reports](https://www.zoho.com/books/help/reports/), [custom reports](https://www.zoho.com/books/help/reports/custom-reports.html) |
| 16 | Books-native projects and time tracking | `full` | Projects, timesheets, and timesheet approvals are native Books capabilities. They are not Zoho Projects and must not be routed as an adjacent product merely because they share names. | [Projects](https://www.zoho.com/books/help/projects/), [timesheets](https://www.zoho.com/books/help/timesheet/), [timesheet approvals](https://www.zoho.com/books/help/timesheet-approval/internal-approval.html) |
| 17 | Users and roles | `full` | Native users/roles and approval-role assignment govern access and participation. The live organization is authoritative for role names and permissions. | [Users and roles](https://www.zoho.com/books/help/settings/users.html), [approval users and roles](https://www.zoho.com/books/help/transaction-approval/users-and-roles.html) |
| 18 | Built-in integrations and online payments | `partial` | Books has documented integrations and payment gateways, but availability and capability vary by product, gateway, country, and organization. Verify the target integration rather than claiming universal coverage. | [Integrations](https://www.zoho.com/books/help/settings/integrations.html), [online payments](https://www.zoho.com/books/help/online-payments/) |
| 19 | Import, export, and backup | `full` | Books provides module import/export and backup flows. Prefer them for supported manual/batch data movement; use API code only when automation or transformation requirements justify it. | [Import and export](https://www.zoho.com/books/help/import-export/), [backup](https://www.zoho.com/books/help/import-export/backup-your-data.html) |
| 20 | Extensions and widgets | `hybrid` | Extensions package Books components and third-party integration; widgets provide developer UI surfaces. They are verified developer surfaces, not standard declarative configuration and not REST/Integration Tasks. | [Extensions](https://www.zoho.com/books/developer/extensions/), [widgets](https://www.zoho.com/books/developer/widgets/) |

## Scope boundaries

- This taxonomy intentionally recognizes broad stable intents, not every toggle, menu, field, lifecycle state, report, gateway, or country rule.
- `none_verified` and `TBD` are required conservative outcomes when a future request has no inspected standard evidence or when dynamic/conflicting evidence prevents classification. They do not authorize guessing.
- Books-native expenses, items, projects, and time tracking remain Books capabilities.
- Zoho Inventory, Expense, Billing, Payments, Payroll, BillPay, and other adjacent products are outside `zoho-books` and outside the initial pack. A Books integration page does not transfer ownership of the adjacent product into this skill.
- Runtime organization metadata, settings, plan/edition, country/region, role permissions, provider availability, and configured connections override static recognition.
- Only plan/edition availability that changes capability selection matters. Volatile numeric quotas, credits, concurrency thresholds, capacities, and timeouts are excluded.

## Advisory wording

Use this non-blocking form before custom generation:

> Zoho Books may provide standard functionality for this intent; verify the target organization's plan, region, permissions, and settings. I will continue with the explicitly requested code unless you choose the standard path.

## Gate checklist

- [x] All 20 required stable intent families are represented.
- [x] `full`, `partial`, `hybrid`, `none_verified`, and `TBD` have deterministic meanings.
- [x] Official Books help/developer URLs support every catalog row.
- [x] Adjacent products and Books-native similarly named capabilities are separated.
- [x] The catalog does not claim an exhaustive UI mirror.
- [x] Runtime plan, region, permissions, providers, and live organization remain authoritative.
