# AI roll-ups

A roll-up buys several small businesses in one trade and runs them as
one. An AI roll-up changes how the work gets done inside them. You buy
a services business at a services price, keep every client, and move
the delivery work to agents. The clients stay. The invoices stay. The
cost of delivering one unit of work drops.

Private equity has run the older version for forty years with
financial engineering: consolidate the back office, squeeze the
vendors, add debt, sell on. The agent version changes the middle step
instead of the balance sheet.

This is doctrine, not a product. It applies to every role, because an
acquired firm arrives with its own books, its own staff and its own
clients, and each role meets it from a different side. Numbers sees a
new ledger. Nectar sees a client list it did not build. Neo sees a
back office nobody automated. The rules below are how the forest takes
on a business it did not grow.

## One firm, one org, one land

An acquired firm is an organization. It gets its own org, its own
land, its own forest and its own NATS account, like every other
tenant. Never a folder inside somebody else's org. Never a shared
database with a tenant column.

This is not tidiness. It is what lets you buy the second firm. Two
firms that share a land share a blast radius, a release cadence and a
credential scope, and the first integration you write between them
becomes the thing you cannot undo. Isolation at the land boundary is
what makes the fifth acquisition the same amount of work as the
first.

## The margin equation

The whole bet is one line, and the forest already computes it weekly
per organization:

```
cost   = expenses + human hours x hourly rate + agent tokens x token rate
margin = invoiced - cost
```

A services firm that runs at five to ten percent margin is the
starting point. Moving work from the first term to the second is the
entire thesis. If human hours fall and margin does not, the thesis is
wrong for that firm and you stop.

Measure it from the start, before you change anything. A baseline you
did not record is a baseline you cannot claim later, to a lender or to
yourself.

## Buy the clients, change the work

The reason to buy instead of build is that the hard parts of a
services business are not software. The client list took thirty years.
The trust is personal. The licences took exams. What "correct" means
in the trade lives in the heads of people who have done it for
decades, and their past work is the only training material that
teaches it.

So the rule is: change the back office, never the relationship. The
margin comes from how the work gets done. The value you paid for is
who trusts whom. Make the relationship feel cheaper and you have sold
the asset to pay for the renovation.

## The rulebook is the asset

Every firm runs on rules nobody wrote down. The client who always
sends documents late. The deduction the senior partner always checks
twice. The phrasing one client finds rude. When the senior people
leave, those rules leave with them.

Getting them out of people's heads and into the forest is the first
real work after a purchase, and it never finishes. Anyone can rent the
same models. Only you hold the list of every way they go wrong in this
trade.

Three rules govern that list:

- **A correction becomes a rule, and a rule becomes a test.** A rule
  that arrived from a real correction carries the input and the
  accepted output with it. Without the test, the next rule silently
  undoes this one.
- **A rule about a client needs a named person's approval.** Agents
  propose. The judgment plane scores. A person at the firm approves,
  through a signed console command. Nothing an agent proposes becomes
  active because the agent was confident.
- **Client identity stays upstream.** The firm's CRM, ledger or
  practice system remains the record for who the client is. The forest
  holds what it concluded about serving them, keyed to the upstream
  identity with its provenance. Two firms with the same client are two
  references, never one silently merged row.

## Agents prepare, people ship

Keep the preparing agent and the reviewing agent apart. The reviewer
can block and may never ship. The preparer ships nothing on its own.
Most bad output never reaches a client because of that one split.

Remove a checkpoint only when the correction rate has earned it, and
only with the number in hand. An agent that can send client mail with
no checkpoint will eventually send the wrong one to the wrong person.

## What breaks

The failures are known and they repeat:

- **Buying faster than you can integrate.** The most common way
  roll-ups fail, with or without agents. Own a pile of messes and the
  model stops being the problem.
- **The key people leave.** The person who knows every client resigns
  six weeks after the sale and the relationships leave with them.
- **Clients leave with the owner.** Some were loyal to a person. Plan
  for churn and make the handover slow and personal.
- **Believing published numbers.** Most reported results in this field
  are self-reported by young firms that are raising money. Underwrite
  on your own measurements.

## Before you buy, in this jurisdiction

The galaxy operates under Belgian and Flemish law, and two gates
decide what can be bought at all.

**Ownership gates.** An ITAA recognised accounting or tax firm must
have the majority of voting rights and a majority of its board held by
ITAA professionals. BIV and IPI apply the same shape to property
managers. A buyer who is not a member cannot take control of these
firms, whatever the price. Trades with no ownership gate include
managed IT services, freight forwarding, and insurance brokerage,
where registration and fit and proper tests apply to the people in
charge rather than to the shareholders.

**Staff transfer.** Under CAO 32bis, staff transfer with their rights
intact on a transfer of undertaking, and dismissal because of the
transfer is prohibited. The margin moves through redeployment, growth
per head and not replacing leavers. Plan in years, not in a hundred
days.

**Financing.** Flemish instruments are built for exactly this size of
deal: a PMV subordinated co-financing loan, the regional guarantee
scheme standing behind a bank credit, and win-win loans from your own
network. A seller note keeps the person who knows the clients invested
in the handover. Match the instrument to the stage; the large PMV
business loans start well above a first acquisition.

Read the local rules before the letter of intent, not after. A gate
you discover in diligence has already cost you the quarter.
