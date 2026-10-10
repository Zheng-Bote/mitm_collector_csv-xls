# Tasks: Bugfix Date Parsing

1. **[x] Task 1:** In `collector-layer/mitm_collector_csv-xls/main.go`, implement a `cleanDate` function to strip single/double quotes.
2. **[x] Task 2:** In `collector-layer/mitm_collector_csv-xls/main.go`, extend `cleanDate` to capture 2-digit years and map them to 4-digit years (Pivot: 26).
3. **[x] Task 3:** Apply `cleanDate` to every value read in the main ingestion loop of `collector-layer/mitm_collector_csv-xls`.
4. **[x] Task 4:** Test compilation.
