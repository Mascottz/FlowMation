# FlowMation data model

I use PostgreSQL as the durable source of truth for FlowMation. The schema keeps workspace ownership, workflow structure, connections, and execution history separate so each area can evolve without making the execution engine responsible for dashboard concerns.

## Core relationships

- A workspace owns users, workflows, and connections.
- A workflow owns ordered workflow steps.
- A workflow run records one attempt to execute a workflow.
- A workflow step run records the result of one step inside a run.
- JSONB payloads keep trigger data and action output flexible while the product is still expanding its connector catalog.

## Local initialization

Docker initializes SQL files mounted into `/docker-entrypoint-initdb.d` on the first database start. To apply this schema manually, I can run:

```bash
psql "$DATABASE_URL" -f database/schema.sql
```

## Status values

Workflow statuses are `draft`, `active`, and `paused`.

Run statuses are `queued`, `running`, `succeeded`, and `failed`.

Connection statuses are `connected`, `expired`, and `revoked`.
