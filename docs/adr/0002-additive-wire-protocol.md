# The wire protocol is versioned and only grows additively

Komari's agent auto-updated into a version its controller rejected, disconnecting every agent at once. Telltale versions the Agent-to-Dashboard protocol in the URL (`/api/v1/...`) separately from release versions. Within a protocol version, fields may only be added — never renamed, removed or redefined; the Dashboard ignores unknown fields and treats missing ones as unknown. A breaking change requires a new protocol version that the Dashboard serves alongside the old one for at least one further major release. The Dashboard never rejects an Agent for being old; it only flags it as outdated. Golden payloads from every released Agent are kept as tests to enforce this.

We accept carrying legacy fields in exchange for upgrades never breaking a fleet.
