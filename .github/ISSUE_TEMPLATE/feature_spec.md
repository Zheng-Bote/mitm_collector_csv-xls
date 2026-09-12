---
name: Feature Specification (Spec Kit)
about: Propose a new feature, collector, or transformation rule.
title: "Feature: "
labels: "feature, spec-kit"
assignees: ""
---

## Feature Intent

_Describe what the feature does and why it is needed. Keep it concise._

## Requirements (EARS Syntax)

1. _Text_
2. _Text_
3. _Text_

## Scope

_What components (Collectors, Transformation, Delivery) does this affect?_

## SpecDD Architecture Alignment (Drift Control)

Please confirm that this feature respects the global `mitm-2` constraints defined in `.sdd` files:

- [ ] **Architecture:** The layered architecture is maintained (e.g. no direct bypass from Collector to Delivery).
- [ ] **Architecture:** Feature affects architecture: SpecKit feature forces update of the SpecDD .sdd
- [ ] **Security:** Envelope Encryption (AES-GCM) is NOT bypassed for PII data.
- [ ] **Data Model:** Core PostgreSQL schemas remain intact (feature-specific tables are allowed).
- [ ] **Standards:** SPDX headers, English documentation, and independent `go.mod` per layer will be maintained.

## Acceptance Criteria

- [ ] _Criterion 1_
- [ ] _Criterion 2_
- [ ] CHANGELOG.md and README.md are up-to-date
