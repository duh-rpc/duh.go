# 1. Record architecture decisions

Date: 2026-06-03

## Status

Accepted

## Context

The project makes architectural decisions whose rationale is easily lost. Months later,
a contributor encounters a constraint or design choice with no record of the forces that
produced it, and risks reversing it without understanding the consequences.

## Decision

We will record architecturally significant decisions as numbered Architecture Decision
Records in `docs/adr/`, following the format described by Michael Nygard. Each record
captures the context, the decision, and its consequences, and is immutable once accepted —
a reversal is a new record that supersedes the old.

## Consequences

- The reasoning behind significant decisions is preserved for contributors who were not
  present when they were made.
- Each decision must be written for a reader with no other context, which takes effort up front.
- Reversing a decision requires a new record rather than an edit, keeping the history auditable.
