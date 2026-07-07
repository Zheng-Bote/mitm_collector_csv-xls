# Changelog

All notable changes to the `mitm_collector_csv-xls` component will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.6.0] - 2026-07-07

### Added
- **SSL Support**: Added support for the `MITM_DB_SSLMODE` environment variable. The collector now respects this setting and applies it to the MitM PostgreSQL connection string.

## [v0.5.0] - 2026-06-30

### Changed
- **Config Restructuring**: Updated database connection setup to accurately parse the JSON configuration (`MITM_DB_CONFIG_JSON`) utilizing the nested `"db"` object format provided by the central scheduler.
- **Database Connection**: The collector now strictly prioritizes the JSON configuration over direct environment variables (`MITM_DB_HOST`, etc.). Direct environment variables are supported as a fallback.
- **Audit Logging**: Added IPC audit logging upon initialization to trace and report the source of the database configuration (`JSON Config (MITM_DB_CONFIG_JSON)` vs `Environment Variables`).

## [v0.4.0] - 2026-06-24

### Added
- **extending logging**


## [v0.3.0] - 2026-06-21

### Added
- **Stateful Aggregation**: Replaced `raw_ingestion_id` with a deterministic `correlation_id` (UUIDv5). 
- **Business Keys**: Introduced a `business_key_column` parameter. The collector now dynamically extracts values from this column (or falls back to the first column) to compute stable correlation IDs. This allows CSV/XLSX fragments to be joined with database fragments in the Transformation Layer.

## [v0.2.0] - 2026-06-15

### Added
- **Centralized App Info**: Added `appName` and `version` globally. The component now broadcasts its name and version via IPC when starting.

## [v0.1.0] - 2026-06-14

### Added
- **File Parsing**: Standalone ingestion tool to parse both `.csv` and `.xlsx` files dynamically.
- **XLSX Support**: Integrated `github.com/xuri/excelize/v2` to support multi-sheet Excel workbooks.
- **Envelope Encryption**: Generates dynamic DEKs, wraps them using `MASTER_KEY` (KEK), and encrypts payload rows using AES-GCM.
- **MitM Landing Zone**: Seamlessly writes encrypted JSON rows, nonces, and `key_id` directly into the MitM PostgreSQL `raw_ingestion` table.
- **IPC Telemetry**: Broadcasts Unix Socket IPC telemetry (`status` and `audit` events) back to the central `mitm_scheduler` for real-time progress tracking in the dashboard.
- **File Cleanup**: Automatically and securely deletes temporary files from the filesystem after processing (or upon failure).
