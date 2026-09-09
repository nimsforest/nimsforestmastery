# Nims and the 5A pattern

A nim is a role in the forest with judgment. It is the only component
that calls a Brain (an AI model). A nim catches Leaves from the Wind,
reads Soil for context, thinks, and answers with new Leaves and
compost. Neo is the developer nim. Nudge is the marketing nim. Nectar
is the sales nim. Each nim is a colleague, not a script.

When you master a role, you learn to BE that nim. You learn its
identity, its behavior, its skills, and its standards. An AI agent
learns the same role from the same sources.

## The 5A pattern

Every nim answers work in one of five shapes, plus Awaken:

1. **Ask.** A pure question and answer. No state changes.
2. **Act.** One discrete operation with a clear result.
3. **Audit.** A review of the domain state, read from Soil.
4. **Automate.** Create a persistent automation that keeps working.
5. **Autonomate.** Change the nim's own behavior. Nims improve their
   own skills overnight through this handler.

**Awaken** is the daily rhythm: the nim processes the dawn and dusk
digests, so it always knows what changed in the forest.

## Boundaries a nim never crosses

- A nim never calls an external API directly. Outbound work goes
  through a Songbird (fire and forget) or the Taproot (guaranteed).
- A nim never writes Soil directly. It emits compost; the Decomposer
  applies it.
- Deterministic transforms belong in TreeHouses, not in nims. If no
  judgment is needed, no Brain is needed.

## Where a role comes from

Everything about a nim lives in the catalog: its identity in
`catalog.nims.<name>`, its behavior prompt in `prompts.nims.<name>`,
its skills and tools in their own catalog entries. This portal
generates your whole path from those live keys. When the nim changes,
your path changes with it.
