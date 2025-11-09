# API & Event Contract Guidelines

## REST

- Prefix all routes with `/v1/{tenant}` to enforce tenant scoping
- Use resource-oriented paths (`/orders/{id}/fulfilment`)
- Employ standard HTTP verbs and status codes; include `X-Request-ID`
- Validate request payloads using DTO structs + validator tags
- Generate OpenAPI specs via `oapi-codegen` or `swag` (tool TBD)

## gRPC / ConnectRPC

- Service definitions stored under `proto/` (to be added)
- Use Buf for linting, breaking change detection, and Go client generation
- Share compiled client libraries with other services (treasury, notifications)

## Events

- Publish domain events to NATS JetStream streams with durable consumers
- Adopt CloudEvents-compatible envelope:

```jsonc
{
  "id": "uuid",
  "source": "food-delivery/orders",
  "type": "order.created",
  "time": "2025-01-01T12:00:00Z",
  "specversion": "1.0",
  "datacontenttype": "application/json",
  "data": { ... }
}
```

- Maintain schemas under `docs/schemas/` and validate using JSON Schema or Avro

## Versioning Strategy

- Backwards-compatible changes only within `v1`
- Introduce new endpoints/fields with default values; avoid breaking renames/removals
- For major breaking changes, launch `/v2` alongside `/v1` with migration plan

## Testing & Governance

- Contract tests executed against API mocks before releasing changes
- API review board approves new endpoints and events to keep parity across services
