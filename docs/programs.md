# Integrating a program with rig

This is the guide for a program author who has never seen rig. It covers what
a program must do to register, answer commands and draw a pane, in Go and in
any other language. The contract behind it is PLAN.md section 5e; this page
is the working summary, and where the two disagree the plan wins and this page
has a bug.

Runnable versions of everything below are in `examples/`.

## What you get

A program connects to `rigd` once, declares what it can do, and from that one
declaration gets a CLI (`rig <program> <command>`), MCP tools for agents, a
window pane and help text. rig never runs code inside your program: the
declaration is data.

## The five steps

1. Connect to the socket.
2. Say hello with your declaration.
3. Answer `<your-id>.ping` (the liveness probe).
4. Answer `<your-id>.<command>` for each command you declared.
5. Stay connected. When the connection closes, rig removes you.

## Go

```go
c, err := client.Connect()   // finds $XDG_RUNTIME_DIR/rig/rigd.sock
defer c.Close()
c.Handle(func(method string, payload []byte) (proto.Message, error) {
    // method is "greeter.ping" or "greeter.greet"
    ...
})
resp, err := c.Hello(ctx, &rigv1.Declaration{...})
<-c.Done()
```

Import `github.com/borismilner/rig/client` and
`github.com/borismilner/rig/proto/rig/v1` (package `rigv1`). A program links
only those two; `proto/rig/v1/registryv1` and `verbsv1` are for rig's own
verbs and a program never needs them. `examples/greeter` is the complete
program, about 150 lines with comments.

Set the handler before `Hello`, or a request can arrive with nothing to
answer it. `Call` makes requests to rig (for example `rig.record.query`);
`Handle` answers requests rig routes to you.

## Any other language

The wire is small enough to speak directly. `examples/python/greeter.py` is a
complete program in about 60 lines using only the proto files and the
`protobuf` package.

**Socket.** A unix stream socket at `$XDG_RUNTIME_DIR/rig/rigd.sock`, mode
0600. There is no fallback path: with `XDG_RUNTIME_DIR` unset, rig and its
clients refuse rather than guess. The whole path must fit in 108 bytes.

**Framing.** Each frame is a 4-byte big-endian length followed by one encoded
`rig.v1.Frame`. A frame over 1 MiB (1,048,576 bytes) is refused and the
connection dropped; a zero length is invalid.

**Messages.** Generate code from `proto/rig/v1/wire.proto` with any protobuf
toolchain (`protoc --python_out`, `prost` for Rust, `ts-proto` or `protobuf-es`
for TypeScript). `wire.proto` is all a program needs; `registry.proto` and
`verbs.proto` are rig's own verbs.

**Streams.** Every request carries a `stream_id`. Streams you open are ODD
(1, 3, 5, ...); rig's are even. A response or error comes back on the stream
it answers. `stream_id` 0 is reserved.

**Hello.** Send a `REQUEST` frame with method `rig.hello` and a `HelloRequest`
payload (`program`, `version`, `declaration`). The answer is a `RESPONSE`
carrying `HelloResponse` (`wire`, `daemon_version`, `scoped`, `session`), or
an `ERROR` whose `status` says exactly what was wrong with the declaration.
One hello per connection.

**Requests to you.** rig sends `REQUEST` frames with method
`<your-id>.<command>`:

| method | payload | answer |
|---|---|---|
| `<id>.ping` | `PingRequest` | `PingResponse` echoing `nonce`, with your id and version |
| `<id>.<command>` | `CallRequest`, `args` is the JSON your schema describes | `CallResponse`, `result` is JSON |

Answer with a `RESPONSE` frame on the same `stream_id`, or an `ERROR` frame
with a `Status`.

## The declaration

`Declaration` carries your identity, your coverage and your commands.

- **Identity**: `id` (lowercase, it is the CLI word and the method prefix),
  `name`, `version`, `description`.
- **Coverage**: `COVERAGE_PARTIAL` with a `coverage_note` saying what you have
  adopted, or `COVERAGE_FULL`. rig tells callers when a picture is partial.
- **semantics_gen**: 1.
- **Commands**: one `Command` per thing a caller can run.

Every `Command` must say, explicitly, `effects`, `idempotent`, `sensitive`,
`interactive`, `streams`, `needs_display`, `duration`, `confirms` and `shape`,
plus `summary`, `description` and `returns`. There are no defaults: every enum's
zero value means "nothing was said" and is refused at registration, so a
command cannot be registered with a property nobody decided. The refusal names
the missing field.

**Arguments.** A command that takes arguments declares a JSON Schema in `args`
(bytes of JSON). Without one, the command takes no arguments and a call that
sends some is refused. `rig <id> <command> --args '<json>'` passes the object
through; your handler receives it in `CallRequest.args`.

**A pane.** Set `pane_url` to an `http://127.0.0.1:<port>/...` URL you serve
(loopback only; anything else is refused at registration) and the window
draws your page. See "Panes" below.

## Errors

Every refusal on the wire is an `ERROR` frame with a `Status`:

| field | meaning |
|---|---|
| `code` | one of the `Code` values below |
| `message` | one sentence saying what happened |
| `precondition` | what had to be true |
| `actual` | what was found instead |
| `fix` | what to do about it |
| `fix_command` | the exact command that does it, when there is one |

Codes: `UNAVAILABLE` (rig not there or stopping), `NOT_FOUND` (no such program,
method or record), `INVALID` (bad frame, argument or enum), `DENIED` (house
rules or a missing seat), `DEADLINE`, `INTERNAL`, `SESSION_DEAD`, `CONFLICT`
(a lost compare-and-swap or a held lease). `UNSPECIFIED` is never sent.

In Go, a failed `Call` returns `*client.CallError`; branch on `Code()`. A
handler refuses a request by returning a `*client.CallError` whose `Status`
it fills in: that Status reaches the caller unchanged. Any other error is
reported as `INTERNAL`, so return a refusal whenever the caller can fix the
problem. `examples/greeter` refuses an empty name this way. In another
language, send an `ERROR` frame with the Status yourself.

## Versioning and compatibility

- The wire is versioned by major: `rig.v1` is the package in the proto files
  and `HelloResponse.wire` says which major the daemon serves. rig serves every
  major it has ever shipped (section 21).
- Within a major, changes are additive only: new fields and new messages, never
  a renumbered or retyped field. `proto/rig/v1/wire_golden_test.go` pins the
  bytes of every message, so a breaking change fails rig's own tests first.
- An enum value your build does not know is a real possibility within a major.
  Treat it as unknown, never as the zero.
- `HelloResponse.methods` lists every `rig.*` method the daemon you reached
  serves, so a program can check for a verb before calling it rather than
  learning its absence from a `NOT_FOUND`.

## Panes

The window shows each program in one of three tiers, decided by your
declaration:

| tier | you declare | you serve |
|---|---|---|
| generated | no `pane_url` | nothing; rig draws your declaration |
| kit | `pane_url` and the kit `elements` you use | your page, composed from `design/kit` (`kit.js`, `kit.css`) |
| embedded | `pane_url`, no `elements` | your page and your own components |

In the kit and embedded tiers your page loads `pane.js` (from `design/kit`,
served by you) and calls `rigPane()`. It receives the window's token set (the
colours, type and spacing as CSS custom properties) and follows theme changes.
`pane.d.ts` beside it gives TypeScript the types. Hide your page until themed
with `html:not([data-rig-painted]) { visibility: hidden }`. `cmd/lantern` is
the smallest embedded-tier program, `cmd/ledger` and `cmd/abacus` the kit
tier, and `tools/f8-demo.sh` shows all three in the window.

## Testing your program

`client/clienttest` starts a real rig daemon inside your test process on a
private socket, so a Go test can register your program and call its commands
through the same wire the CLI uses, with nothing installed. See its godoc and
`examples/greeter`'s test. Any language can instead start `rigd` with a
private `XDG_RUNTIME_DIR` and `XDG_STATE_HOME` (two `mktemp -d` directories),
which is how `tools/f8-demo.sh` does it.

## Lessons

rig keeps an estate-wide store of lessons (PLAN.md section 40): something one
seat learned that another should not have to learn again. A program reaches it
with `Call`, like any other rig verb:

| method | request | answer |
|---|---|---|
| `rig.knowledge.search` | `query`, `limit` (1 to 20, default 5) | hits: id, title, summary, snippet, score. Never a body |
| `rig.knowledge.get` | `id` | the whole lesson and who wrote it |
| `rig.knowledge.add` | `title`, `summary`, `body`, `tags` | the lesson as stored |

Search before a long investigation and fetch only the hit that fits. The
words in `query` are matched as words; there is no query syntax. A write
needs a seat (`rig.announce` first; a terminal has one) and is attributed to
it. At a terminal the same verbs are `rig knowledge search|get|add`; agents
get `knowledge_search`, `knowledge_get` and `knowledge_add`.

## Agents

Nothing extra is needed: every declared command reaches agents through rig's
MCP door (`list`, `describe`, `invoke`, `query`), and a command with
`promote` set becomes its own MCP tool.
