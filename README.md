# FlowMation

I am building FlowMation as a calm, premium workflow automation platform for teams that want powerful automation without a noisy interface.

## Product direction

FlowMation lets me create workflows that connect the tools I already use, run repeatable tasks, and keep a clear history of every execution. The experience should feel precise, quiet, and trustworthy.

## Current milestone

I have started with the first dashboard direction:

- Premium workspace overview
- Active workflow summary
- Task, time saved, and success rate metrics
- Recent activity feed
- Usage tracking
- Responsive layout for desktop and mobile
- Inline SVG icon system with no external asset dependency
- Calm navy, sage, lavender, sand, and coral color system

## API reference

I keep the first gateway contract in `API.md` so the frontend, Go gateway, and Rust engine can evolve against the same request shapes.

## Data foundation

I have added the first PostgreSQL schema in `database/schema.sql`. It covers workspaces, users, workflows, ordered workflow steps, connections, workflow runs, and per-step execution records.

The gateway now includes a PostgreSQL storage adapter in `services/gateway/storage.go`. It is ready to be wired into the request handlers when `DATABASE_URL` is present, while the in-memory store remains available for fast interface development.

I can create local environment values from `.env.example` before starting the database services. The first Docker database start also creates a demo workspace, owner, and lead enrichment workflow from `database/seed.sql`.

## Planned architecture

- `frontend`: FlowMation dashboard prototype
- `dashboard`: PHP application for authentication, workflows, billing, and workspace management
- `services/gateway`: Go API gateway and request boundary
- `services/engine`: Rust execution engine for safe, fast workflow steps
- PostgreSQL for durable application data
- Redis for queues, rate limiting, and transient workflow state
- A Go background worker for queued workflow validation

## Local preview

I can preview the first dashboard without a build step:

```bash
cd frontend
python3 -m http.server 4173 --bind 0.0.0.0
```

Then I can open `http://localhost:4173`.

## Product voice

FlowMation should sound clear, considered, and useful. I avoid inflated technical language, unnecessary urgency, and clutter. Every screen should help me understand what is happening and what I can do next.

## Attribution

Powered by MASTECH INNOVATIONS, info@mastechinnovations.com.ng, +2349138825300
