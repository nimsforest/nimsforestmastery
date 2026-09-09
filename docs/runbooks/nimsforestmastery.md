# Runbook: deploy and operate nimsforestmastery

The role mastery portal. One instance per organization at
`mastery.<org>.mynimsforest.com`. The nimsforest org instance carries
the alias `developer.nimsforest.com`. Design authority:
nimsforest2/docs/architecture/ROLE_MASTERY.md.

## What it serves

- Human surface (nwc pages): `/` role directory (public), `/doctrine`
  shared curriculum (public), `/roles/<nim>` Meet, `/roles/<nim>/learn`
  and `/roles/<nim>/changelog` behind iamnim SSO.
- Agent surface: `/roles/<nim>.md`, `/llms.txt`, `/api/v1/roles`,
  `/api/v1/roles/<nim>/bundle`, behind iamnim PAT Bearer auth.
- `/health` and `/static/` are public.

## Environment variables

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `ORG_SLUG` | yes | none | Single tenancy. The service refuses to start without it. |
| `PORT` | no | `8111` | HTTP listen port. |
| `NATS_URL` | no | `nats://127.0.0.1:4222` | The forest bus. Unreachable NATS degrades, it does not stop the service. |
| `IAMNIM_URL` | for auth | none, on purpose | Identity service. Empty means every authenticated route answers 503, fail closed. |
| `BASE_URL` | for SSO | none | The portal's own external URL, the iamnim login return target. Empty means the human role pages answer 503. |
| `NIMREGISTRY_URL` | no | `http://127.0.0.1:8101` | HTTP fallback read path when Soil is not readable. |
| `CONTENT_DIR` | no | `./content` | On-disk shared doctrine content. The image sets it to `/usr/share/nimsforestmastery/content`. |
| `NATS_CREDENTIALS` | no | none | Path to a NATS .creds file, honored by the standard connect chain. |

Flags mirror the variables (`-org`, `-port`, `-nats`, `-iamnim`,
`-nimregistry`, `-base-url`, `-content`); the environment wins, with
one exception: `ORG_SLUG` is the tenancy authority and the `-org`
flag may only agree with it. A `-org` value that disagrees with
`ORG_SLUG` stops the service at startup instead of being overridden.

## Land role shape

Accretion tier, per-org land, behind the land reverse proxy:

```yaml
  nimsforestmastery:
    lifecycle: persistent
    network: host
    port: 8111
    domains:
      - mastery.{{.OrgSlug}}.mynimsforest.com
    env:
      NATS_URL: nats://127.0.0.1:4222
      PORT: "8111"
      ORG_SLUG: "{{.OrgSlug}}"
      IAMNIM_URL: "https://iamnim.com"
      BASE_URL: "https://mastery.{{.OrgSlug}}.mynimsforest.com"
      NIMREGISTRY_URL: "http://127.0.0.1:8101"
```

No volumes: the portal stores no role content and no state. The
doctrine content ships inside the image.

## Deploy

Always CI/CD to registry to land pull; never a manual build.

1. Tag and push:
   ```
   git -C ~/nimsforestmastery tag v0.1.0
   git -C ~/nimsforestmastery push origin v0.1.0
   ```
2. GitHub Actions runs tests, publishes release binaries, and pushes
   `registry.nimsforest.com/nimsforestmastery:<version>` and `:latest`
   through the OIDC docker login.
3. Verify the tag exists in the registry, then plant on the land:
   ```
   land plant nimsforestmastery --config /etc/land.yaml
   ```

## Seeding

Nothing to seed. Role content is live: every role page and bundle is
generated at request time from the org's own Soil, with the local
nimregistry HTTP API as the fallback. The only content in the image is
the shared doctrine curriculum (a COPY of `content/`), which is
role-agnostic and release-coupled until the #257 hub pull adopts the
pre-declared `catalog.mastery.doctrine.*` keys.

## Docs slot

The bundle's docs index carries relative links on the portal's own
authenticated routes (the role page, the bundle, the changelog, the
doctrine), so out-of-forest consumers can follow them. Per-role
runbook links are DEFERRED: their live source is `catalog.docs.*`
(#92), which does not exist yet. When #92 lands, the docs slot adopts
those keys behind the same bundle shape.

## Health

`GET /health` is public and answers the nimsforesttool JSON shape
with per-check detail:

- `soil`: the Soil read path. In the disabled state the check says why
  and names the fallback.
- `nimregistry`: the fallback API's own `/health`.
- `iamnim`: reachability. An anonymous 401 from iamnim counts as
  healthy; a network failure or server error does not.

Liveness is separate from degradation. The status code answers "can
this instance serve roles at all", so an orchestrator can gate on it:

- All checks pass: 200 `status: ok`.
- Some check fails but a role read path remains (for example only the
  unused nimregistry fallback is down, or iamnim is down): 200
  `status: degraded` with the per-check reasons. A restart would not
  help, so the instance is not reported dead.
- Both the Soil read path and the nimregistry fallback fail: 503
  `status: unavailable`. No role read path is left.

The joining contract runs alongside: register on
`forest.mycelium.register`, 30s heartbeats, deregister on shutdown.

## Disabled and degraded modes

The portal never serves stale renders and never opens up:

- NATS or the Soil bucket unreachable: the service starts, logs a
  warning, and reads through the nimregistry fallback. Renderings are
  marked degraded, carry revision 0 stamps, and say that tool and
  skill assignments are missing because they live only in Soil.
- Both Soil and nimregistry unreachable: role routes answer an honest
  503 with the reason. The doctrine pages keep serving read-only.
- `IAMNIM_URL` or `BASE_URL` missing: authenticated routes answer 503.
  There is no fallback allow.
- iamnim unreachable at request time: 503 "identity service
  unavailable", never an allow and never a cached identity.
- Soil watches fail to start: the render cache is disabled and every
  request reads through to Soil. Slower, but no stale render can be
  served silently.

## Auth model

- Humans: iamnim SSO only (the console rule, #238). Every protected
  request re-checks `GET /api/me` plus `GET /api/me/memberships` and
  requires `ORG_SLUG` in the membership list. No cache. The login
  callback token moves into the `mastery_session` cookie (Secure,
  HttpOnly, SameSite=Lax) and leaves the URL.
- The callback is state bound: the login redirect sets a random
  short-lived `mastery_login_state` cookie and puts the same state in
  the `redirect_uri` query, which iamnim echoes back. A `?token=`
  without a matching state is ignored and a fresh login starts, so a
  crafted link cannot push a session onto a browser. An existing
  valid session cookie always wins over a query token, so a garbage
  `?token=` cannot evict a valid session.
- Agents and CLIs: iamnim PATs (`inpat_`), sent as `Authorization:
  Bearer`, validated with the same two-call fail-closed check.
- The portal mints no tokens and holds no static secrets.
