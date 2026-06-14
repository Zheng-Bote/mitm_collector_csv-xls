# Changelog

All notable changes to the `mitm_collector_csv-xls` component will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.1.0] - 2026-06-14

### Added
- **File Parsing**: Standalone ingestion tool to parse both `.csv` and `.xlsx` files dynamically.
- **XLSX Support**: Integrated `github.com/xuri/excelize/v2` to support multi-sheet Excel workbooks.
- **Envelope Encryption**: Generates dynamic DEKs, wraps them using `MASTER_KEY` (KEK), and encrypts payload rows using AES-GCM.
- **MitM Landing Zone**: Seamlessly writes encrypted JSON rows, nonces, and `key_id` directly into the MitM PostgreSQL `raw_ingestion` table.
- **IPC Telemetry**: Broadcasts Unix Socket IPC telemetry (`status` and `audit` events) back to the central `mitm_scheduler` for real-time progress tracking in the dashboard.
- **File Cleanup**: Automatically and securely deletes temporary files from the filesystem after processing (or upon failure).
