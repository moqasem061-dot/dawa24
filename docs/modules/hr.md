# Module: Human Resources (HR)

## Overview

The `hr` bounded context manages organization employee rosters, compensation structures, and branch weekly operating schedules.

## Schema Mapping

- **PostgreSQL Schemas:** `hr`
- **Migrations:** `011_hr.up.sql`
- **Tables Owned:**
  - `hr.employees` — Staff profiles and salary compensation.
  - `hr.work_times` — Weekly business hours and shift schedules.

## Invariants & Rules

1. **Exact Salary Calculations:** Salaries use `money.Amount` minor unit arithmetic.
2. **Tenant Isolation:** Staff rosters and branch work schedules are enforced per-tenant with `FORCE ROW LEVEL SECURITY`.
