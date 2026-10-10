# Architecture Plan: Bugfix Date Parsing

## Reference
- mitm-2.sdd (Global Architecture)
- Feature Spec: bugfix_date_parsing.md

## Design
The `mitm_collector_csv-xls` is a source collector. It currently reads string values and puts them into a `map[string]string` without sanitization. 
To fix the date parsing bug (where `"22.02.92"` or `22.02.92` becomes `22.02.0092`), we need to introduce a cleaning function inside `main.go`.

### Execution Plan
1. **Value Sanitization**: Before storing a value in `recordMap[headers[j]]`, we will trim surrounding quotes (`'` and `"`).
2. **Date Normalization**: We will use a regular expression or a heuristic to detect dates in the format `DD.MM.YY` (or `DD.MM.YYYY`). If a 2-digit year date is found, we will parse it.
    - If the year is between 00 and 99, we need to map it to a 4-digit year. For example, years > 50 map to 19XX, and years <= 50 map to 20XX, or we can use Go's `time.Parse` behavior. Wait, Go's `time.Parse("02.01.06", ...)` handles the 2000/1900 pivot (usually values 69-99 are 19xx, 00-68 are 20xx). We will normalize this to `DD.MM.YYYY` so downstream does not get confused.

## Constraints Check
- No secrets are written to disk.
- PII handling remains the same (still encrypted via `encryptAESGCM` before DB insert).
- Only changes in `mitm_collector_csv-xls`.
