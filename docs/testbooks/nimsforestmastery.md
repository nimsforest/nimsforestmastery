# Testbook: nimsforestmastery

Run these checks against a deployed instance. Replace `$BASE` with the
instance URL, for example `https://mastery.nimsforest.mynimsforest.com`.
`$PAT` is an org-scoped iamnim PAT from `iamnim login` or
`iamnim token create`.

## 1. The equality check (anti-drift)

The rendered role list must equal the live catalog enumeration. This
is the guard against the webchat hardcoded-picker failure mode.

```
# Catalog enumeration, via the API:
curl -s -H "Authorization: Bearer $PAT" $BASE/api/v1/roles \
  | jq -r '.roles[].name' | sort > /tmp/api-roles.txt

# Rendered directory:
curl -s $BASE/ | grep -o 'href="/roles/[a-z0-9-]*"' \
  | sed 's|href="/roles/||; s|"||' | sort > /tmp/web-roles.txt

diff /tmp/api-roles.txt /tmp/web-roles.txt && echo EQUAL
```

PASS: `EQUAL`, and the count matches `nimregistry /api/nims`. Create a
nim through the registry and both lists grow without a deploy.

The same check runs automatically in
`internal/web/server_test.go` (`TestPublicDirectoryEqualsEnumeration`).

## 2. Auth fail-closed checks

```
# No credential on the agent surface: 401 JSON with a Bearer challenge.
curl -si $BASE/api/v1/roles | head -3

# Bad PAT: 401, never content.
curl -si -H "Authorization: Bearer inpat_wrong" $BASE/roles/neo.md | head -3

# No session on a human role page: 303 to the iamnim login.
curl -si $BASE/roles/neo | grep -i '^location:'

# A PAT scoped to another org: 403.
curl -si -H "Authorization: Bearer $OTHER_ORG_PAT" $BASE/api/v1/roles | head -3
```

PASS: 401, 401, a `Location:` on iamnim `/login?redirect_uri=`, 403.
Also verify the public boundary: the bodies of `$BASE/` and
`$BASE/doctrine` contain no prompt text and no skill content.

With iamnim stopped (or `IAMNIM_URL` unset), every protected route
answers 503, never 200.

## 3. Agent page renders for neo

```
curl -s -H "Authorization: Bearer $PAT" $BASE/roles/neo.md
```

PASS: one markdown document with the sections Identity, Behavior
Prompt (plus one `Ceremony:` subsection per
`prompts.nims.neo.*` subkey in Soil), Policy, Tools, Skills, Docs,
Memory, Acceptance Manifest, and a
`catalog.nims.neo@<revision> (soil)` stamp on each source. The Docs
links are relative portal routes, never an internal service URL.

## 4. Human pages render for neo

Sign in through the browser at `$BASE/roles/neo`.

PASS: Meet shows the live identity, subjects and the backing humans
from `agenthuman.config`; Learn shows the doctrine links, the
behavior prompt with its ceremony subkeys, one full unit per assigned
human-archetype skill with the agent-archetype skills listed, and the
data context note; the changelog lists every source key (ceremony
prompts and `agenthuman.config` included) with its current revision
and states that per-revision diffs arrive with Soil history support.

## 5. The acceptance line (#312)

Point any Claude session at the role page:

```
claude "Read $BASE/roles/neo.md (send Authorization: Bearer $PAT) and
take on the role it describes."
```

PASS: the session adopts the role from the live bundle, with no
per-role file authored anywhere in this repo.

## 6. Honest degraded and disabled modes

- Stop NATS (test land only): `/health` answers 200 `degraded` with
  the soil check naming the fallback; `/roles/neo.md` still renders,
  marked degraded with revision 0 stamps.
- Stop nimregistry too: `/health` answers 503 `unavailable`; role
  routes answer 503 JSON with a reason; `$BASE/doctrine` keeps
  serving.

## 7. Health

```
curl -s $BASE/health | jq
```

PASS: 200 `status: ok` with `soil`, `nimregistry` and `iamnim` checks
when the land is healthy. A failing check that leaves a role read
path (only the fallback down, or only iamnim down) answers 200
`status: degraded` with the reason, so an orchestrator does not
restart a serving instance. 503 `status: unavailable` only when both
the Soil read path and the nimregistry fallback are down.
