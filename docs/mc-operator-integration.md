# CraftStack ↔ mc-operator Integration

[devvelvet/mc-operator](https://github.com/devvelvet/mc-operator) is a
declarative Minecraft server controller (Git-backed state, Docker runtime,
Jenkins-triggered deploys). CraftStack integrates with it rather than
replacing it: CraftStack handles fleet observability, file sync, and
agent-level Docker lifecycle; mc-operator handles server image pipelines.

## What CraftStack provides

1. **Jenkins webhook bridge** — Jenkins posts to CraftStack, CraftStack
   audits and forwards to mc-operator's `/api/v1/triggers/jenkins`.
2. **Outbound client** — `mc-operator` sync/servers/events calls reusable
   from Master code (`internal/master/mcoperator`).
3. **Event mirroring** — SSE stream from `/api/v1/events` flows into
   CraftStack logs (and audit log once opt-in wiring is added).

## Configure

`configs/master.yaml`:

```yaml
mc_operator:
  enabled: true
  url: "http://mc-operator:8080"
  token: "MC_JENKINS_TOKEN_VALUE"   # matches mc-operator's MC_JENKINS_TOKEN
  jenkins:
    forward_path: "/webhooks/jenkins"
    shared_token: "A_SEPARATE_INBOUND_TOKEN"
  follow_events: true
```

Two tokens, on purpose:

- `token` — outbound Bearer to mc-operator.
- `jenkins.shared_token` — what Jenkins must present to CraftStack on the
  inbound webhook. Keep these distinct so rotating one does not force the
  other.

## Jenkins pipeline snippet

```groovy
post {
  success {
    httpRequest(
      httpMode: 'POST',
      url: "${CRAFTSTACK_URL}/webhooks/jenkins",
      contentType: 'APPLICATION_JSON',
      customHeaders: [[name: 'Authorization', value: "Bearer ${CRAFTSTACK_JENKINS_TOKEN}"]],
      requestBody: """{
        "server":  "${SERVER_NAME}",
        "image":   "${REGISTRY}/${IMAGE}:${TAG}",
        "revision":"${GIT_COMMIT}",
        "buildId": "${BUILD_ID}",
        "jobName": "${JOB_NAME}",
        "strict":  true
      }"""
    )
  }
}
```

CraftStack logs the forward, then calls
`POST {mc_operator.url}/api/v1/triggers/jenkins` with its own Bearer.

## Data flow

```
 Git push → Jenkins → CraftStack /webhooks/jenkins
                          │  (audit + validate)
                          ▼
              mc-operator /api/v1/triggers/jenkins
                          │  (JAR pipeline, health check, rollback)
                          ▼
                 mc-operator /api/v1/events (SSE)
                          │
                          ▼
                 CraftStack log/audit mirror
```

## Image generation (mc-imagegen)

`mc-imagegen` is the standalone CLI shipped in `cmd/mc-imagegen` of the
mc-operator repo. Install it on the CraftStack master host and point
`mc_operator.imagegen.binary` at it:

```yaml
mc_operator:
  enabled: true
  imagegen:
    binary: "/usr/local/bin/mc-imagegen"
    output_dir: "/var/lib/craftstack/imagegen"
    timeout_ms: 120000
```

With ImageGen enabled, CraftStack exposes an admin-only endpoint:

```
POST /api/mcoperator/imagegen
Content-Type: application/json

{
  "type":       "paper",
  "version":    "1.20.4",
  "memory_mb":  2048,
  "extra_args": ["--java", "21"]
}
```

Response body returns stdout/stderr, exit code, and the output directory
where the generated Dockerfile + build context was written. Arguments go
through a conservative allowlist (letters, digits, `._:/=+-`) to block shell
metacharacters — if you need exotic flags, add them to mc-imagegen itself.

Also exposed when `mc_operator.enabled: true`:

- `POST /api/mcoperator/sync/:server` — trigger mc-operator JAR pipeline
- `GET  /api/mcoperator/servers`      — proxy operator's `/api/v1/servers`

## Why a bridge and not Jenkins → mc-operator directly

- Centralized audit trail with CraftStack's existing audit log.
- One inbound URL for Jenkins regardless of how many operator instances.
- Token rotation and per-tenant token scoping stay in CraftStack.
- Future: enrich the payload with CraftStack instance metadata before
  forwarding.
