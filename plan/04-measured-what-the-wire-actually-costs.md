## 4. Measured: what the wire actually costs

Run on this laptop, 2026-09-10, Go 1.27.1, two real processes. The full harness is in the repo
as `cmd/ipcbench` so the numbers can be re-taken on any machine.

| Transport | Per operation | Rate |
|---|---|---|
| Direct Go function call (what an imported library gives) | 0.6 ns | 1.6G/s |
| Unix socket round trip, 64 B payload | **6.2 µs** | 161k/s |
| Unix socket round trip, 4 KB | 7.8 µs | 128k/s |
| Unix socket round trip, 64 KB | 19.0 µs | 53k/s |
| Round trip with JSON encode and decode, 81 B | 11.4 µs | 87k/s |
| One-way socket write, 256 B log record | **561 ns** | 1.8M/s |
| Shared memory, spin-wait round trip | 143 ns | 7.0M/s |

**What this decides.**

- The daemon architecture costs nothing that matters. A config read at startup, a notification, a
  command invocation, a health check: all of them at 6µs, thousands of times over, is noise.
- **Logging does not need shared memory.** A plain socket write is 561ns and sustains 1.8M
  records per second. Nothing here will produce a thousandth of that.
- **Shared memory is not built.** It is 43x faster and there is no workload that needs it. If one
  ever appears, the number above is what justifies adding it, and not before.
- A binary codec is worth having over JSON (5µs of the 11.4µs is the codec), but it is a
  preference, not a requirement.
- **gRPC is not bought.** Its server on a unix socket measured **+9.80 MiB resident** for
  HTTP/2 framing, flow control and stream machinery that a single local socket does not need.
  The framing this plan actually requires is a length prefix, and §17 is where that 9.80 MiB
  went. Protobuf stays; gRPC goes.

---
