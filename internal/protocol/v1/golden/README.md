# Golden payloads

One directory per released Agent version, holding exactly what that Agent
sends: `report.json` and `host_info.json`. Tests check that

- every payload still decodes strictly into today's protocol v1 types, so no
  field has been renamed, removed or changed type;
- the Dashboard's ingest API accepts every payload;
- every field today's Agent can send appears in at least one payload, so a
  new field cannot ship without being recorded here.

When releasing, copy the newest directory to one named after the release and
update it to match what the new Agent sends. Never edit or delete a released
version's files. See `docs/protocol.md`.
