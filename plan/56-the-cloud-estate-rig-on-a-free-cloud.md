## 56. The cloud estate: rig on a free cloud host, Marvin on Telegram

**Requirement, Boris, 2026-10-10, verbatim** (said in the
buddies-cloud-plan seat's session in `~/me/d2d`):

> I want us to write the infrastructure to be used by the cloud services
> to interact and do everything they need to do. I don't know if it
> should be part of `rig` or should we develop a dedicated `rig` for
> cloud.

> I agree to do it as recommended - how about we call the bot Marvin
> after the robot in the famous book?

What "as recommended" was, and is now ruled:

| Ruling | Meaning |
|---|---|
| **one rig, not a cloud fork** | the cloud host runs this rig as a third estate, `cloud`, beside production and development (section 37) |
| headless | the cloud rigd runs no tray, no window, no speech, no hand; nothing in it reaches the laptop |
| shared by every cloud service | storage, timers, queues, leases, supervision, health, events, the record: what rig already is, used by every program on the host instead of each building its own |
| four additions, built in rig | below |
| the host is not rig's | provisioning, hardening and secrets live in `~/me/projects/cloudhost` |

Binding constraints from the same day (cloudhost SPEC.md):
- **absolutely free**: Oracle Always Free only, zero spend beyond the
  Claude subscription;
- **never asks**: a cloud Claude Code run cannot stall on a question,
  a prompt, a login or stdin;
- **the laptop reaches in, never the reverse.**

### The four additions

| # | Addition | Shape |
|---|---|---|
| 1 | **a headless profile** | rigd started with the desktop modules off; `notify` and toasts route to Telegram instead of the tray; `rig health` says which modules are off rather than reporting them broken |
| 2 | **Claude Code runs as a rig capability** | `rig claude run` (and the matching MCP verb): a transient systemd unit with a time limit, fixed never-ask flags, the subscription token from a secret file, no `ANTHROPIC_API_KEY`, a 5h-window deferral, run slots, a full record. Starts from cloudhost's `internal/run` (commit f7d5579). buddies moves onto it later instead of keeping its own |
| 3 | **Marvin, the Telegram module** | one bot, named **Marvin** (Boris: after the robot in *The Hitchhiker's Guide to the Galaxy*). Sends what `notify` routes to it; long-polls for his commands (`/status`, `/runs`, `/ask`); answers only his chat id; no webhook, no open port. Starts from cloudhost's `internal/telegram` |
| 4 | **the laptop reaches the cloud rig** | an ssh-forwarded unix socket (`ssh -L <local.sock>:<remote rigd.sock>`), so `rig --estate cloud ...` on the laptop talks to the cloud rigd with no network code in rig. Started only from the laptop |

Decided on his behalf, open to change:
- **Marvin's voice is plain in alerts.** A dry Marvin line is fine in
  `/start` and `/status` small talk; anything a service sends goes out
  as written, because an alert must be read at a glance.
- **What crosses Telegram is a short title**; detail stays on the host
  or the laptop. Mom's rule (no "oncology" in names, titles or
  headlines) binds every sender.

Checked 2026-10-10: `rigd` and `rig` build for linux/arm64 with
`CGO_ENABLED=0` (28 MB, 12 MB). The tray and window are `rigwindow`,
a separate binary.

Open, to be checked live on the host: whether `claude -p /usage` works
under a `setup-token` login; whether rigd's user service starts clean
with no session bus or display (linger, no GNOME).

### Measured on the host, and two rulings that replace parts of the above (2026-10-10)

Measured with a real rigd on the cloudhost VM (x86_64, no desktop),
built `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`:

- **rigd already runs headless.** Store, notify, the record and the
  event bus work; `toast.posted` is published with nobody drawing it.
  It degrades by itself and says so in its log: `rig is silent
  component=audio`, `rig.desk is off: no screen lock to read`. `rig say`
  answers `CODE_UNAVAILABLE` cleanly. So addition 1 needs no new
  profile in rigd: the desktop modules already turn themselves off.
- **The laptop reaches the host's rig with no rig code**: ssh forwards
  the host's `rigd.sock` and `rigd-mcp.sock` into a directory of their
  own (`/run/user/<uid>/cloud/rig/`), and the laptop's `rig` runs with
  `XDG_RUNTIME_DIR` pointed there. `rig estate` then answers
  `root /home/ubuntu/.rig`. `RIG_SOCKET` does NOT steer the client
  (tried: it reached the laptop's own daemon). Addition 4 is done by
  configuration.

**Ruled by Boris, 2026-10-10** (asked as two questions):

| Question | Ruling |
|---|---|
| the host's estate name | **`production`, on its own host.** Section 37's closed set (production, development) is unchanged: it is one production per machine. The laptop's link names it "cloud"; no core change. This replaces "a third estate, `cloud`" above |
| where the Claude runner and Marvin live | **rig programs in `~/me/projects/rigged`**, like buddies and storeworker: `rig claude run ...`, `rig marvin ...`. rig's core is untouched; each module is its own program, and any service reaches them through rig. This replaces "four additions, built in rig" above for additions 2 and 3 |

What is left for rig's core from this section: nothing required.
Gaps met while building the programs are fixed in rig as they are met
(rigged SPEC, 2026-10-04).
