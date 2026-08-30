# Zoho CRM standard module baseline

This catalog recognizes standard Zoho CRM module display names and API names. It is an official-evidence-filtered baseline, not a globally exhaustive module inventory and not a write-safety, permission, layout, relationship, quota, or runtime-policy catalog.

The source snapshot was generated from authenticated Zoho CRM V8 Modules Metadata (`GET /crm/v8/settings/modules`) on 2026-08-29, then filtered against the official standard CRM evidence registered in [the evidence dossier](zoho-crm-evidence-dossier.md). Runtime metadata remains authoritative for the target organization.

Module API name is authoritative. Display names are canonical where the registered official evidence proves them. An observed tenant alias is included only when it helps recognize a renamed standard module; it is not canonical.

| Canonical Display Name | Module API Name | Observed Tenant Alias |
|---|---|---|
| Leads | `Leads` | — |
| Accounts | `Accounts` | Clients |
| Contacts | `Contacts` | — |
| Deals | `Deals` | Opportunities / Projects |
| Campaigns | `Campaigns` | — |
| Tasks | `Tasks` | — |
| Cases | `Cases` | — |
| Meetings | `Events` | — |
| Calls | `Calls` | — |
| Solutions | `Solutions` | — |
| Products | `Products` | — |
| Vendors | `Vendors` | — |
| Price Books | `Price_Books` | — |
| Quotes | `Quotes` | — |
| Sales Orders | `Sales_Orders` | PreInvoices |
| Purchase Orders | `Purchase_Orders` | — |
| Invoices | `Invoices` | — |
| Appointments | `Appointments__s` | — |
| Services | `Services__s` | — |
| Notes | `Notes` | — |
| Attachments | `Attachments` | — |

## Scope boundary

The filter deliberately excludes tenant-specific custom modules, extension- or tenant-generated entities, custom subforms, linking modules, field trackers, web tabs, integration surfaces, dashboards, and other organization-specific entries. Neither `generated_type != custom` nor `generated_type == default` proves that a module is standard.
