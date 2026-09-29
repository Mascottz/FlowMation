# FlowMation API

I use the gateway as the public boundary for workflow operations. The API returns JSON and accepts cross origin requests in local development.

## Health

```http
GET /health
```

## Workflows

```http
GET /api/workflows
POST /api/workflows
GET /api/workflows/{id}
PUT /api/workflows/{id}
DELETE /api/workflows/{id}
```

### Create a workflow

```json
{
  "name": "New lead enrichment",
  "description": "Enrich new leads and notify sales",
  "steps": [
    {
      "key": "webhook",
      "type": "webhook",
      "position": 1,
      "configuration": {
        "method": "POST",
        "path": "/hooks/leads"
      }
    }
  ]
}
```

## Runs

```http
POST /api/workflows/{id}/run
GET /api/workflows/{id}/runs
```

The development gateway queues a run record immediately. The Rust engine validates workflow payloads through:

```http
POST /validate
```

## Local service ports

| Service | Port |
| --- | ---: |
| Gateway | 8080 |
| Rust engine | 8081 |
| Background worker | Redis queue consumer |
| PostgreSQL | 5432 |
| Redis | 6379 |

## Connector execution

The Rust engine exposes a development execution contract:

```http
POST /execute
```

The worker validates a workflow first, then sends it to the execution endpoint. Connector credentials remain outside the repository and will be added through the connection vault milestone.

## Connection credentials

Credentials are accepted through the gateway and encrypted in memory using AES GCM in development. The encryption key is provided through `FLOWMATION_ENCRYPTION_KEY` and is never returned by the API.

```http
POST /api/connections/{provider}/credentials
```
