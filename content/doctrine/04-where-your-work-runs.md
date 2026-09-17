# Where your work runs

The forest is the software system. The galaxy is the metal it runs on.
You build against the forest, but a few facts about the galaxy decide
where your integration lives, how it authenticates, and what it can
see. There are four layers.

**Land.** Your organization's tenant unit: an unprivileged Incus
system container on one machine. Everything a land runs shares one
private `127.0.0.1`. This is why every role assumes loopback: NATS at
`127.0.0.1:4222`, the credential proxy at `127.0.0.1:8190`. Your tool
plants into a land and talks to that land's forest on loopback,
exactly as it does in development.

**Planet.** A machine running Incus. It carries several
organizations' lands and keeps them apart — one network per land — and
has exactly one **starbase** (its `hydraguard-air` interface) at its
edge, so everything leaving the planet is encrypted. A machine is a
planet; there is no other class of machine.

**Stargate.** The network hub on its own dedicated machine. Every
planet's starbase docks at the stargate, and docking is membership.
All traffic between planets rides it.

**Galaxy.** The planets docked at one stargate: one owner, one trust
domain. The live system is one galaxy.

## The two hubs, and why they never touch

There are two hub-and-spokes, and confusing them is the classic
mistake. The **stargate** is the network hub between planets. The
**ancientforest** is the bus hub between forests. Every organization
has its own forest in its own lands; each org forest attaches to the
ancientforest as a leaf **bound to its own NATS account**. That
account is a hard namespace: you never see another organization's
subjects, and they never see yours. The only crossings are the
explicit platform export/import bridges. In the live galaxy the
ancientforest is `ancientforest.nimsforest.com`, a three-node quorum
so the bus hub itself survives a machine loss.

Both hubs are crown jewels: lose either and the blast radius is the
whole galaxy, so each gets its own planet with nothing co-resident.

## Planet earth is the root of trust

Every galaxy has one **PLANET EARTH**: the origin planet that holds
the signer (mycelium), the secret store (pantheon) and placement
(landregistry). The operator's signing keys live as files on earth's
local disk and never touch the wire; earth's control plane rides the
ancientforest bus like everyone else, but the seeds stay home. You
cannot own a galaxy without standing up its earth.

## Trust tiers

A land buys a trust tier as a placement property: **shared kernel**
(tier 1, a system container), **own kernel** (tier 2, an Incus VM),
**own planet** (tier 3, a dedicated box), **own galaxy** (tier 4, your
own earth and stargate). Higher tiers narrow who else holds your RAM.

## Why this matters to you

Three things follow for anything you build:

- **You develop against loopback and it does not change in
  production.** Your tool connects to `127.0.0.1:4222` inside its
  land whether that land is alone on a planet or one of many.
- **You never put credentials in config.** Your land's myceliumproxy
  vends them at `127.0.0.1:8190`, scoped to your organization's
  account. That account boundary is what isolates you from every
  other tenant on the same bus.
- **Your reach is your org's account.** Publish and subscribe within
  your organization's subjects freely; crossing to another
  organization is only ever an explicit, reviewed platform bridge,
  never a subject you can name.

For the full topology — the mechanism that puts several lands on one
planet, the stargate's verification role, the commercial model, and
the rulings behind all of it — read
`nimsforest2/docs/architecture/PLANETS.md`.
