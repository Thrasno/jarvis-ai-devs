# Zoho Deluge — Scripting Standards

## Critical Rules

### No Nested Loops
Never nest `for` loops in Deluge. Each API call inside a loop = one API credit. Zoho imposes hard limits.

```deluge
// BAD: nested loop hits API limits
for each record in records {
    for each item in record.get("items") {
        // API call inside nested loop = rate limit death
    }
}

// GOOD: flatten data before iterating
allItems = list();
for each record in records {
    allItems.addAll(record.get("items"));
}
for each item in allItems {
    // single level, controlled
}
```

### No API Calls Inside Loops

Collect IDs first, then bulk fetch:

```deluge
// BAD
for each id in contactIds {
    contact = zoho.crm.getRecordById("Contacts", id);
}

// GOOD
contacts = zoho.crm.searchRecords("Contacts", "id:in:" + contactIds.toString(","));
```

### Null Safety with ifnull()

Always use `ifnull()` before accessing nested fields:

```deluge
// BAD — crashes if field is null
email = record.get("Email").toLowercase();

// GOOD
email = ifnull(record.get("Email"), "").toLowercase();
```

### Early Returns

Use `return` to exit early on invalid conditions:

```deluge
if (customerId == null || customerId == "") {
    return {"status": "error", "message": "Customer ID is required"};
}
```

### Bulk Operations

Prefer `zoho.crm.bulkUpdate()` over single-record updates in loops. Maximum 100 records per bulk call.

## Common Patterns

### Safe Map Access
```deluge
value = ifnull(myMap.get("key"), defaultValue);
```

### Date Formatting
```deluge
today = zoho.currentdate;
formatted = today.toString("yyyy-MM-dd");
```

### Error Handling
```deluge
response = zoho.crm.createRecord("Leads", data);
if (response.get("code") != "SUCCESS") {
    // handle error
    return {"status": "error", "details": response};
}
```
