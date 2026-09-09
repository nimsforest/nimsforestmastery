# What a forest is

An organization has lands. A land is a server that runs containers and
services. Each land grows a forest: an embedded NATS server with
JetStream. All forests in the organization connect through NATS
clustering. Together they form one system.

The forest is not a central server. It is the living system that
appears when lands connect.

## The parts you will meet

**Leaf.** An immutable event record. Every Leaf has a subject, JSON
data, a source, and a timestamp. Leaves are the only way events move.

**Wind.** The pub/sub layer over NATS Core. Leaves travel on the Wind.
A component drops a Leaf to publish it and catches a subject pattern
to receive.

**River.** A durable stream for raw inbound data. External systems
push into the River. Trees parse River data into typed Leaves.

**Soil.** The shared key-value store that holds current state. One
bucket, one truth, shared across all lands. Any component can read
Soil. Only the Decomposer and Fossil write it.

**Humus.** The durable stream of state changes. A component that wants
to change state emits compost into Humus. The Decomposer applies the
compost to Soil with optimistic locking. This single writer prevents
concurrent overwrites.

**Taproot.** The durable outbound dispatcher. Writes that must reach
an external system leave the forest as tap Leaves. Rootlets deliver
them with retries and record the outcome.

**Bedrock.** Persistent file storage: git repositories, filesystems,
Google Drive. Soil holds metadata; Bedrock holds the files. Never
store secrets in Bedrock.

## Why this matters to you

Every role in the forest works through these parts. You do not call a
service directly. You drop a Leaf, read Soil, or emit compost. The
data flow rules protect the whole organization: when you follow them,
your work is observable, replayable, and safe.
