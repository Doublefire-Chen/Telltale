# The Agent–Dashboard protocol

Agents talk to the Dashboard over a small, versioned HTTP protocol. Its
compatibility rules are what let you upgrade the Dashboard without touching a
single Agent, and upgrade Agents one Host at a time, in any order
([ADR 0002](adr/0002-additive-wire-protocol.md)).

## Protocol v1

- `POST /api/v1/host-info` — the Host's static description (Host Info).
- `POST /api/v1/report` — one Report of changing metrics.

Both take a JSON body of at most 64 KiB and `Authorization: Bearer <Host token>`.
The Dashboard answers with a status code and nothing else — `204` accepted,
`400` malformed, `401` unknown token, `429` too many rejected requests — and
the Agent reads nothing else ([ADR 0001](adr/0001-agent-is-one-way.md)).

The types are defined once, in `internal/protocol/v1`, and shared by both
binaries.

## Rules for changing v1

Within v1 the format only grows.

1. **Add, never change.** You may add a field. You may not rename, remove or
   change the type or meaning of an existing one.
2. **New fields are optional.** Use a pointer (or `omitempty` string) so that
   "not sent" is distinguishable from zero. Older Agents will never send it.
3. **The Dashboard ignores what it does not know** and shows what is missing
   as unknown ("—"). Never make a new field required for a Report or Host
   Info to be accepted.
4. **Never reject an Agent for being old.** Flag it as outdated instead.
5. **Record every field in the golden corpus.** `internal/protocol/v1/golden`
   holds what each released Agent sends. Tests fail if a recorded payload no
   longer decodes strictly (something was renamed or removed), if the
   Dashboard does not accept one, or if the Agent can send a field that no
   payload records.

### When releasing

Copy the newest directory in `internal/protocol/v1/golden/` to one named after
the release, and make its files match exactly what the new Agent sends. Never
edit or delete the files of a version that has been released.

## When v1 is not enough: v2

A change that cannot be made additively (a field must change meaning, say)
needs a new protocol version:

1. Add `internal/protocol/v2` and endpoints under `/api/v2/`.
2. Keep serving `/api/v1/` alongside it, unchanged, for at least one more major
   Dashboard release — in practice for as long as it is cheap.
3. New Agents speak v2 only to Dashboards that support it; an older Dashboard
   must not break them. Document how before shipping.
4. Start a golden corpus for v2; v1's stays and keeps running.

Breaking the protocol breaks fleets. Exhaust additive options first.
