# Types, Conversions, and Dates

Verified: 2026-08-16 against the official Deluge documentation. Every URL below was requested and answered on that date.

Sources:

- Text functions index: https://www.zoho.com/deluge/help/functions/text.html
- Date-time functions index: https://www.zoho.com/deluge/help/functions/date-time.html
- Number functions index: https://www.zoho.com/deluge/help/functions/number.html
- Logical functions index: https://www.zoho.com/deluge/help/functions/logical.html
- `isNull`, `isBlank`, `isEmpty` difference: https://www.zoho.com/deluge/help/functions/common/isnull-isblank-isempty-difference.html
- `toDate`: https://www.zoho.com/deluge/help/functions/common/todate.html
- `toString`: https://www.zoho.com/deluge/help/functions/common/tostring.html

## `isNull` versus `isBlank` versus `isEmpty`

Three checks, three different questions. Picking the wrong one is a silent bug.

| Check | True when | Page |
|---|---|---|
| `isNull(value)` | The value is null — never assigned, or explicitly null | https://www.zoho.com/deluge/help/functions/common/isnull.html |
| `isBlank(value)` | The value is null, an empty text, or text made only of whitespace | https://www.zoho.com/deluge/help/functions/common/isblank.html |
| `isEmpty(value)` | The text or collection holds nothing | https://www.zoho.com/deluge/help/functions/text/is-empty.html |

Practical rules:

- Guarding user or external text: `isBlank`. It covers null and `"   "` in one check.
- Distinguishing "not provided" from "provided as empty": `isNull`.
- Deciding whether to enter a loop over a Map or List: `isEmpty`.

```deluge
if(isBlank(inputName))
{
	return {"ok": false, "reason": "name is required"};
}
if(isEmpty(rows))
{
	return {"ok": true, "processed": 0};
}
```

## Never chain onto an unguarded value

A text or date function applied to null fails at runtime. Guard first, or supply a default with `ifNull`.

```deluge
// Fails when the key is absent.
slug = payload.get("title").toLowerCase();

// Guarded.
title = ifNull(payload.get("title"), "");
if(isBlank(title))
{
	return {"ok": false, "reason": "title is required"};
}
slug = title.trim().toLowerCase().replaceAll(" ", "-");
```

## Text functions worth knowing

`toLowerCase()` and `toUpperCase()` — note the capital C and U inside the names.

```deluge
normalized = rawText.trim().toLowerCase();
parts = fullName.toList(" ");
found = subject.contains("invoice");
piece = reference.subString(0, 4);
clean = body.replaceAll("\n", " ");
```

Pages: https://www.zoho.com/deluge/help/functions/string/trim.html · https://www.zoho.com/deluge/help/functions/string/tolist.html · https://www.zoho.com/deluge/help/functions/string/substring.html · https://www.zoho.com/deluge/help/functions/string/replaceall.html

Read the casing off the page heading and the syntax line, never off the URL. Documentation slugs are lowercased, so `substring.html` documents `subString`. `leftpad`, by contrast, really is all lowercase on its own page.

`leftpad` is all lowercase and pads with whitespace only. It takes a single argument — the total number of whitespaces — and there is no pad-character parameter. Do not reach for it to zero-pad a number.

```deluge
padded = "Left".leftpad(6);
// returns "  Left"
```

https://www.zoho.com/deluge/help/functions/string/leftpad.html

## Conversions

Convert only after validating the input. A conversion on unvalidated text is a runtime failure waiting for the first bad record.

```deluge
rawAmount = payload.get("amount");
if(isBlank(rawAmount))
{
	return {"ok": false, "reason": "amount is required"};
}
amount = rawAmount.toDecimal();
quantity = ifNull(payload.get("quantity"), "0").toLong();
label = amount.toString();
```

Pages: https://www.zoho.com/deluge/help/functions/common/todecimal.html · https://www.zoho.com/deluge/help/functions/common/tolong.html

There is no documented boolean-conversion function. Compare the text explicitly instead of inventing one:

```deluge
isActive = (ifNull(payload.get("active"), "false").toLowerCase() == "true");
```

`toDate` parses an expression into a date-time value. It has both a method form and a standalone form, and it takes an optional date-time mapping when the expression is text:

```deluge
dueDate = "2026-08-16".toDate();
startedAt = toDate("16/08/2026", "dd/MM/yyyy");
```

The mapping uses `y` for the year, `M` for the month, and `d` for the day of the month, separated exactly as the input text separates them. A numeric month must be written `M` or `MM`; a textual month needs `MMM` or longer.

## Dates

`today` returns the current date with the time set to 00:00:00; `now` returns the current date-time. Both are documented date-time members, so prefer them.

```deluge
currentDate = today;
currentMoment = now;

formatted = currentDate.toString("yyyy-MM-dd");
nextWeek = currentDate.addDay(7);
lastMonth = currentDate.subMonth(1);
elapsed = daysBetween(startDate, currentDate);
```

Pages: https://www.zoho.com/deluge/help/functions/datetime/today.html · https://www.zoho.com/deluge/help/functions/datetime/now.html · https://www.zoho.com/deluge/help/functions/datetime/addday.html · https://www.zoho.com/deluge/help/functions/datetime/daysbetween.html · https://www.zoho.com/deluge/help/functions/datetime/days360.html

Other documented members on the date-time index: `addMonth`, `addYear`, `addWeek`, `addHour`, `addMinutes`, `addSeconds`, `addBusinessDay`, the matching `sub*` members, `getDay`, `getMonth`, `getYear`, `getHour`, `getMinutes`, `getSeconds`, `getWeekOfYear`, `getDayOfYear`, `monthsBetween`, `hoursBetween`, `yearsBetween`, `toStartOfMonth`, `toStartOfWeek`, `weekday`, `workday`, `unixEpoch`.

Use `weekday` for the day of the week. Anything absent from that index is unverified — confirm it or ask the user.

## Time zones and formats are environment facts

`toString` on a date-time takes an optional format and an optional time zone, and its default format comes from the service settings, not from the language. When a computation depends on the time zone or on a display format the application controls, confirm it with the loaded `zoho-[app]` skill, or ask the user. Do not assume UTC.

## Comparing dates

Compare date values directly; do not compare their formatted text.

```deluge
if(dueDate < today)
{
	return {"ok": false, "reason": "due date is in the past"};
}
```
