# Collections — Map and List

Verified: 2026-08-16 against the official Deluge documentation. Every URL below was requested and answered on that date.

Sources:

- List functions index: https://www.zoho.com/deluge/help/functions/list.html
- Collection functions index: https://www.zoho.com/deluge/help/functions/collection.html
- `containKey` (KEY-VALUE): https://www.zoho.com/deluge/help/functions/map/containkey.html
- `containsKey` (COLLECTION): https://www.zoho.com/deluge/help/functions/collection/containskey.html
- `put` (KEY-VALUE): https://www.zoho.com/deluge/help/functions/map/put.html
- `toString`: https://www.zoho.com/deluge/help/functions/common/tostring.html

There is no combined Map index page. Map members are documented one page at a time under `functions/map/`; confirm the individual page before relying on a member this file does not list.

## Declaring collections explicitly

Declare the collection before filling it. An explicit declaration makes the shape obvious to the reader and to the next function.

```deluge
contacts = Map();
contacts.put("name", "Ada Lovelace");
contacts.put("email", "ada@example.com");

pending = List();
pending.add("first");
pending.add("second");
```

Literal forms are equivalent and read well for small fixed shapes:

```deluge
result = {"ok": true, "count": 2};
codes = {"alpha", "beta"};
```

## Map access

`get` returns null for an absent key, so guard before chaining anything onto the result.

```deluge
rawEmail = payload.get("email");
if(isNull(rawEmail))
{
	return {"ok": false, "reason": "email is missing"};
}
email = rawEmail.trim().toLowerCase();
```

`ifNull` supplies a default in one expression. The capital N is part of the name.

```deluge
email = ifNull(payload.get("email"), "").trim().toLowerCase();
```

## Key checks: `containKey` versus `containsKey`

These are two different functions on two different types, and the distinction matters.

| Function | Type it belongs to | Question it answers |
|---|---|---|
| `containKey(key)` | KEY-VALUE (Map) | Does this map have that key? |
| `containsKey(key)` | COLLECTION | Does this collection contain that key? |

For a Map, the key check is `containKey`.

```deluge
if(!payload.containKey("email"))
{
	return {"ok": false, "reason": "email key absent"};
}
```

`containKey` distinguishes an absent key from a key present with a null value; `isNull(map.get(key))` cannot. Use `containKey` when that difference is meaningful, and `ifNull` when it is not.

Other documented Map members, each with its own page under `functions/map/`: `put`, `putAll`, `get`, `containValue`, `keys`, `size`, `remove`, `clear`, `isEmpty`.

## List operations

Documented on the List index: `add`, `addAll`, `get`, `contains`, `notContains`, `indexOf`, `lastIndexOf`, `remove`, `removeAll`, `removeElement`, `clear`, `size`, `isEmpty`, `sort`, `distinct`, `intersect`, `subList`, `toJSONList`, `toList`.

```deluge
items = List();
items.add("one");
items.addAll(otherList);

if(items.contains("one"))
{
	position = items.indexOf("one");
}

items.removeElement("one");
total = items.size();
```

`add` appends a single element; `addAll` appends every element of another list. Adding a list with `add` nests it instead of flattening it.

`toString` converts an expression to text. It takes no separator argument, so do not join a list by passing one: build the joined text explicitly when you need it.

```deluge
joined = "";
for each entry in items
{
	if(joined != "")
	{
		joined = joined + ", ";
	}
	joined = joined + entry;
}
```

## Iteration

```deluge
totals = Map();
for each item in items
{
	key = item.get("category");
	if(isBlank(key))
	{
		continue;
	}
	running = ifNull(totals.get(key), 0);
	totals.put(key, running + 1);
}
```

Filter and deduplicate before iterating, not inside the loop body:

```deluge
unique = rawValues.distinct();
```

## Sorting and set operations

- `sort(ascending)` orders a list: https://www.zoho.com/deluge/help/functions/list/sort.html
- `distinct()` removes duplicates: https://www.zoho.com/deluge/help/functions/list/distinct.html
- `intersect(list)` returns the shared elements: https://www.zoho.com/deluge/help/functions/list/intersect.html
- `subList(from, to)` slices: https://www.zoho.com/deluge/help/functions/list/sublist.html
- `removeElement(value)` removes by value: https://www.zoho.com/deluge/help/functions/list/removeelement.html

There is no documented `union` member. To combine two lists, use `addAll` followed by `distinct()`.

If a member is not on the index above and has no page of its own, treat it as unverified: confirm it against the documentation for the application you target, or ask the user. Do not use it because it looks plausible.
