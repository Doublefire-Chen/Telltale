# Agents send Reports by HTTPS POST, not a persistent stream

Nezha (gRPC bidirectional streams) and Komari (WebSocket JSON-RPC) both stream from the agent, and in both the same channel carries remote commands, terminals and updates back to the agent. Telltale's Agent instead POSTs one Report every interval (default 2s, minimum 1s) over HTTPS with HTTP/2 (falling back to HTTP/1.1) and reads only the status code. With HTTP/2 header compression the bytes on the wire are nearly identical to WebSocket, the Agent needs only the Go standard library for transport, and there is no bidirectional channel for features to creep onto. Liveness comes from timestamps (Offline after three missed intervals) rather than connection state — which Nezha also does in practice. The Dashboard still pushes to browsers over WebSocket, since that leg is where "live" matters.

## Considered Options

- **WebSocket with a "close on any inbound data frame" guard** — instant disconnect detection, but more code (reconnect, heartbeats) and a two-way channel by design.
- **HTTP/3** — no benefit for small periodic requests on a long-lived connection, needs quic-go, and UDP is often blocked or throttled on VPS and cross-border paths. Can be added later as an opt-in since the request semantics are unchanged.
