# nimsforestmastery

Role mastery paths for humans and AI agents. One mastery path per nim,
generated live from the artifacts that already run the nim. One
instance per organization at `mastery.<org>.mynimsforest.com`; the
nimsforest org instance is neoportal at `developer.nimsforest.com`.

Design authority: nimsforest2/docs/architecture/ROLE_MASTERY.md.
Issues #309 (MASTER), #312 (this service), #302 (tracking) on
issues.nimsforest.mynimsforest.com.

## The two surfaces

One source, two renderings. Every page and bundle is generated at
request time from the org's own Soil (fallback: the local nimregistry
HTTP API), stamped with the revision of each source key.

**Human surface** (nwc pages):

- `/` the live role directory (public). The list IS the catalog
  enumeration; no role is authored here.
- `/doctrine` and `/doctrine/<unit>` the shared curriculum (public),
  loaded from on-disk markdown in `content/doctrine/`.
- `/roles/<nim>` Meet, `/roles/<nim>/learn` Learn and
  `/roles/<nim>/changelog`, behind iamnim SSO (fail closed, no cache,
  org membership required on every request).

**Agent surface** (markdown and JSON, iamnim PAT Bearer auth):

- `/roles/<nim>.md` the whole path as one markdown document; point a
  Claude session at it to take on the role.
- `/llms.txt`, `/api/v1/roles`, `/api/v1/roles/<nim>/bundle` (the
  six-slot bundle: identity, policy, skills, memory, docs,
  acceptance).

`/health` is public and serves the nimsforesttool health shape.

## Environment variables

| Variable | Purpose |
| --- | --- |
| `ORG_SLUG` | Required. Single tenancy; the service refuses to start without it. |
| `PORT` | Listen port, default `8111`. |
| `NATS_URL` | The forest bus, default `nats://127.0.0.1:4222`. Unreachable NATS degrades to the nimregistry fallback. |
| `IAMNIM_URL` | Identity service. No default on purpose; empty fails closed with 503. |
| `BASE_URL` | This portal's external URL, the login return target. Empty fails the human role pages closed. |
| `NIMREGISTRY_URL` | Fallback read path, default `http://127.0.0.1:8101`. |
| `CONTENT_DIR` | Doctrine content directory, default `./content`. |

See docs/runbooks/nimsforestmastery.md for deploy and operations and
docs/testbooks/nimsforestmastery.md for the acceptance checks.

## Public repo boundary

This repository is public. It holds code, HTML templates, and the
role-agnostic doctrine curriculum. It holds NO role content, NO
credentials, and NO org data: role pages and bundles exist only at
request time, behind authentication, generated from the org's own
forest. Never commit a secret, a token value, or org-specific content
here.

## Develop

```
make build   # bin/nimsforestmastery
make test
make vet
ORG_SLUG=nimsforest ./bin/nimsforestmastery -nats nats://localhost:4222
```

Releases: tag `v<version>`, push the tag; CI publishes binaries and
the image on registry.nimsforest.com; deploy with
`land plant nimsforestmastery --config /etc/land.yaml`.
