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

## 1b. The doctrine enumeration check

The rendered curriculum must equal the units on disk. Same anti-drift
guard as check 1, for the only content this repo ships.

```
# On disk, in the image:
ls /usr/share/nimsforestmastery/content/doctrine/*.md \
  | xargs -n1 basename | sed 's|^[0-9]*-||; s|\.md$||' | sort > /tmp/disk-units.txt

# Rendered index:
curl -s $BASE/doctrine | grep -o 'href="/doctrine/[a-z0-9-]*"' \
  | sed 's|href="/doctrine/||; s|"||' | sort -u > /tmp/web-units.txt

diff /tmp/disk-units.txt /tmp/web-units.txt && echo EQUAL
```

PASS: `EQUAL`, and every slug answers 200 on `$BASE/doctrine/<slug>`.
There is no unit list in the code, so a new file is the whole change.

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

- Stop NATS (test land only): `/health` answers 503 `degraded` with
  the soil check naming the fallback; `/roles/neo.md` still renders,
  marked degraded with revision 0 stamps. A degraded portal is still
  serving, so nothing restarts it: the land role declares no health
  block.
- Stop nimregistry too: `/health` stays 503 `degraded`, now with both
  `soil` and `nimregistry` failing, which is how you tell a portal
  serving through the fallback from one with no read path left; role
  routes answer 503 JSON with a reason; `$BASE/doctrine` keeps
  serving.

## 7. Health

```
curl -s $BASE/health | jq
```

PASS: 200 `status: ok` with the `soil`, `nimregistry` and `iamnim`
checks all `ok` when the land is healthy.

There are two states, not three: `nimsforesttool`'s shared handler
answers 200 `ok` when every check passes and 503 `degraded` when any
check fails. Any single failure, including iamnim alone, is a 503.
Severity is not lost, because the per-check detail names the path that
failed. Read the detail, never the status code alone: a portal still
serving through the fallback and a portal with no read path left both
answer 503.
