# Standard resource and field recognition

This recognition-only baseline comes from the approved 2026-08-31 42-file OpenAPI snapshot. Runtime organization metadata, settings, plan, region, permissions, and configured providers are authoritative. It contains no custom or tenant-specific fields. Standard envelope property names such as `custom_fields` are API structure, not tenant field definitions.

## Resource groups

| Resource identity | Official title |
|---|---|
| `bank-accounts` | Bank Accounts |
| `bank-rules` | Bank Rules |
| `bank-transactions` | Bank Transactions |
| `base-currency-adjustment` | Base Currency Adjustment |
| `bills` | Bills |
| `chart-of-accounts` | Chart Of Accounts |
| `contact-persons` | Contact Persons |
| `contacts` | Contacts |
| `credit-notes` | Credit Notes |
| `currency` | Currency |
| `custom-modules` | Custom Modules |
| `customer-debit-notes` | Customer Debit Notes |
| `customer-payments` | Customer Payments |
| `delivery-challans` | Delivery Challans |
| `estimates` | Estimates |
| `expenses` | Expenses |
| `fixed-assets` | Fixed Assets |
| `integration` | Zoho CRM Integration |
| `invoices` | Invoices |
| `items` | Items |
| `journals` | Journals |
| `locations` | Locations |
| `opening-balance` | Opening Balance |
| `organizations` | Organizations |
| `pricelists` | Price Lists |
| `projects` | Projects |
| `purchase-order` | Purchase Order |
| `recurring-bills` | Recurring Bills |
| `recurring-expenses` | Recurring Expenses |
| `recurring-invoices` | Recurring Invoices |
| `registers` | Registers |
| `reporting-tags` | Reporting Tags |
| `retainer-invoices` | Retainer Invoices |
| `sales-order` | Sales Order |
| `sales-receipt` | Sales Receipt |
| `tasks` | Tasks |
| `taxes` | Taxes |
| `time-entries` | Time Entries |
| `transaction-locking` | Transaction Locking |
| `users` | Users |
| `vendor-credits` | Vendor Credits |
| `vendor-payments` | Vendor Payments |

## Schema and field closure

The snapshot contains 4,671 component schemas and 2,527 inline roots, for 7,198 total roots. It closes at 20,987 flattened field/structure rows with 20,987 unique file/root/field identities. Reconciliation found zero unresolved references, zero cycles, and zero malformed records. All 1,084 valid schema markers reconcile with zero reconciliation defects.

Preserve file-local identity `(source YAML, root identity, field path)`, request versus response reachability, requiredness within the immediate schema, references, composition, array/item and map structure, formats, enum/default values, nullable, read-only, and write-only markers. A response field does not imply writability, and snapshot absence does not prove runtime non-support. Resolve the selected operation's accepted request and observed response from its current official contract and runtime facts.
