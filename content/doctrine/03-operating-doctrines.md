# Operating doctrines

These rules apply to every role, human or agent. They are how the
forest stays observable, repeatable, and safe.

## Issue first

File an issue before or alongside every non-trivial change. The
tracker is the shared memory of the organization. Work that starts
without an issue is invisible work, and invisible work drifts.

## Runbook culture

Every deliverable ships with a runbook: how to deploy it, operate it,
and verify it. A testbook records how to prove it works. If only one
person can operate a thing, the thing is not done. Write the runbook
so a colleague can follow it without you.

## Operate through the forest

State lives in Soil. Events travel as Leaves. Writes leave as taps and
compost; the Decomposer is the only Soil writer. Do not reach around
the forest with direct edits, side channels, or private copies of
state. What the forest cannot see, the forest cannot keep true.

## Never manual

Deploys go from a version tag through CI to the registry, and the land
pulls the image. Never build on a server. Never run a container by
hand. Never edit configuration files on a server. A manual step is a
step nobody can repeat, audit, or roll back.

## The console rule

Every console is reachable through a domain with iamnim sign-in.
Never a tunnel, never a console only one machine can reach. Access
must survive any single laptop, and it must be revocable in one place.

## No static tokens

Long-lived secrets do not belong in configuration, code, or skills.
People sign in through iamnim. Services receive short-lived
credentials, vended per organization through mycelium. A leaked static
token stays valid until someone remembers it exists; a vended token
expires on its own.
