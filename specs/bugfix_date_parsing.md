# Feature Spec: Bugfix Date Parsing in CSV/XLS Collector

## Problem
Dates in Excel or CSV files are sometimes formatted with 2-digit years and optionally wrapped in quotes (e.g., `"22.02.92"` or `22.02.92`). The collector currently passes these values directly as strings. If a downstream system or another layer tries to parse this, or if the collector itself is responsible for normalizing dates, a 2-digit year like `92` can end up being misparsed as `0092` (e.g., `22.02.0092`).

## Scope
`collector-layer/mitm_collector_csv-xls`

## Acceptance Criteria
- When reading cell values from CSV or Excel, any surrounding single or double quotes must be stripped.
- If a value represents a date with a 2-digit year (e.g., `DD.MM.YY`), it should be parsed and normalized to a 4-digit year format (e.g., `DD.MM.YYYY` or ISO8601).
- Values that are not dates should remain unaffected.
