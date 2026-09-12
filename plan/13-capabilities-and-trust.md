## 13. Capabilities and trust

Deny by default. A program declares what it needs at registration, rig enforces it, and every
denial is logged with a trace id and shown.

| Capability | Enforced by |
|---|---|
| `secrets` | Keyring reads scoped to the named keys, namespaced per program |
| `storage` | A path the program cannot escape |
| `network` | An allowlist, via a network namespace where available, otherwise at the proxy and honestly labelled best-effort |
| `filesystem` | Declared paths only, mode-checked, landlock where the kernel supports it |
| `subscribe` | The bus refuses a subscription without a grant |
| module-contributed | Every service and surface registers the capability names it grants (§5h). The kernel checks a grant against a set it never had to know in advance - the previous enum literally listed `tray`, `notify`, `schedule`, three module names inside the kernel |

Capabilities are enforced where enforcement is real and labelled best-effort where it is not; the
UI says which per program. Widening a capability needs an explicit confirmation, so an edited
manifest cannot quietly gain access. **Widening `sensitive` needs the same confirmation**, since
after §15 that declaration is security-relevant.

**A hosted program (§5j) gets in-process enforcement only.** Path confinement, network
namespaces and landlock are per-process instruments and there is no second process. The Programs
view says so per program rather than implying a boundary that is not there.

### 13a. house rules - the authorization floor

**There is no authorization language anywhere else in this plan.** rig carries `effects` and a
danger level from the registry, and the principal from the connection, and before this section
it never put the two together. Its answer used to be §9's "an agent can refuse or confirm on its
own" - which is the restraint living inside the thing being restrained.

`house rules` is a small table: which kinds of caller may run which kinds of command, and which
must ask first.

```
[[rules]]
caller  = "agent"           # agent | terminal | window | script | program
                            # | schedule | bus | url | any
effects = "destructive"     # at least as dangerous as this, per the declared property
action  = "confirm"         # allow | confirm | deny
```

**`effects` in a rule is a floor, not an equality.** It matches a command whose declared
effects are **at least as dangerous** as the value named, so a rule written against
`writes-files` covers `destructive` too. Equality was the first reading and it cannot express
the one default this section ships: a rule naming `read-only` would match read-only calls and
nothing else, so the dangerous call - the only call the rule exists for - falls through to the
empty set and is allowed. The kernel already orders the enum, for exactly this.

**When more than one rule matches, the most restrictive wins: `deny` over `confirm` over
`allow`.** The table needs this the day it ships, because the `url` default below is two rules
that both match the same call. First-match makes the outcome depend on the order lines happen
to sit in a file, and most-specific makes a narrow `allow` beat a broad `deny`; both fail open,
and this is the authorization floor.

**The four clauses below were unanswered until 2026-09-11**, and the fourth is
an authority hole that was **demonstrated in this tree, not inferred from a term
sweep.**

| Clause | What it requires |
|---|---|
| **Evaluated on the exact effective arguments that will execute** | with no re-parsing between the decision and the dispatch. §13a matches on the pair `(caller, effects)` and arguments were never matched at all. A decision taken against one argument set and dispatched with another has authorised something nobody decided |
| **Evaluation cannot error and cannot block, and any failure is `deny`** | a rules table that throws is a table that fails open. This is the same reasoning that refused first-match and most-specific above, applied to the evaluator rather than to the ordering |
| **A decision names its matching rule, its config layer, and the remedy** | `origin` carries `rule`/`elevation` and the rule id today. **The layer and the remedy are not recorded**, so an operator reading a refusal cannot tell which file to edit |

#### A program must not be able to weaken its own effects unobserved

**`effects` is declared by the program, and house rules match on that
declaration.** So a rule written as `(agent, destructive) -> confirm` stops
matching the moment that command presents itself as `writes-files`. **No rule is
violated and no denial is logged**, because the pair simply stopped matching.
`effects` being *"a floor, not an equality"* widens it: one step down the enum
drops every rule written at or above the old level.

**The reachable path is a RECONNECT, and this was corrected on 2026-09-11 after
being got wrong.** The first statement of this hole said a live session
re-registers and the entry is replaced. **That is true of the kernel and not
reachable from the daemon**, which refuses a second handshake on a live
connection outright. The path that actually exists is ordinary: the connection
closes, the close deregisters the program, and it reconnects declaring the same
command one level weaker. **A deploy is enough. No hostile program and no race.**

**That makes it harder to close, not easier**, and the difficulty is the
specification: the old declaration is gone by the time the new one arrives, so
**closing this means remembering a declaration past the connection that made
it.** A comparison at registration time has nothing to compare against unless
rig keeps the prior declaration deliberately.

**The observability half is separate, and nothing covers it today.** rig logs
**decisions**, not changes of declaration. There is no event class for *"the
pair this command presents has changed"*, so an operator watching for refusals
sees only a call that was allowed. The restart appears in the log as a
deregistration and a fresh registration, and **neither line says that a rule
which had been gating that command no longer reaches it.**

**Both halves are required.** Detecting the weakening without emitting an event
leaves the operator blind; emitting the event without keeping the prior
declaration leaves nothing to emit.

#### No path to invocation may drop the caller, and a signature that omits it is the defect

**§13a's floor lives in the kernel's invoker for one stated reason: "no surface
can forget it, and a surface added in 2028 is covered by rules written in
2026."** That guarantee is structural, and it holds only while every path to an
invocation still carries a principal when it arrives.

**Found 2026-09-11 while building M2's meta layer: an internal interface between
the meta tools and the daemon took the program, the command and the arguments -
and no principal.** The layer above it had one and used it for a visibility
check, then dropped it. **So whatever implemented that interface was handed
nothing to authorise with.** The floor was not bypassed; it was made
*unreachable* by a type signature.

**This is the sharpest form of the defect §13a exists to prevent**, and it is
worth stating as its own rule because it does not look like a security bug from
either side: the caller believed it had authorised, the implementer had nothing
to authorise with, and no rule was violated because no rule was consulted.

| The rule | |
|---|---|
| **Every function on a path to invocation carries the principal.** A signature that cannot express the caller is a defect in the signature | An interface whose shape makes forgetting compulsory defeats a floor that exists so nobody has to remember |
| **One authorisation path, not one per surface.** A new surface SPLITS the existing path and reuses its floor; it never grows a second one | Two floors is how they diverge, and the one that diverges is whichever was added last |
| **A surface that mints a principal is walked against §14's caller table as part of the change** | already §14's rule, and this is the case it was written for |

**Why it was not exploitable when it was found, stated so the fix is not
mis-timed:** at M2 the only caller through that layer connects over the socket
as an ordinary unregistered client and gets everything §14's table grants such a
client. **One principal, no scoping, no gap.**

**The fuse is the HTTP surface.** §14 specifies an HTTP client as *"its bearer
principal's scopes, never `introspect`"* - a genuinely narrower principal, and
the first one that would reach an invocation with its scope dropped. **So this
is settled BEFORE that surface is built, not during it**, which is what §14's
walk-the-table rule already requires.

#### The confirmation channel is out of band from the caller

**§14 and §13a route `confirm` to a window, toast or terminal and bind the
answer to "that one call, not the connection". That is the hard half and it is
right.** The clauses below are the rest, and each was checked against the client
surface on 2026-09-11 rather than assumed.

| Clause | State today |
|---|---|
| **No argument, flag, header or second call can satisfy a confirmation** | see the four channels below. **Three exist unreserved and one does not exist at all** |
| **Deny when no human is reachable. Not queue, and not wait** | absent. A confirmation that queues until somebody appears is an approval with a delay on it |
| **Approval is bound to one invocation by id and argument digest** | **NOTHING BINDS IT, AND THE MECHANISM DOES NOT EXIST - CORRECTED 2026-09-11.** `request_id` is on the wire and the daemon copies it onto the reply. **There is no dedup anywhere in the tree**: no map, no cache, no window, no replay check. No client or CLI sets the field either. §5f specifies the window as **persisted in the WAL**, and the WAL lands at M7, so this clause cannot be satisfied before M7. **The superseded text said "the daemon dedups against a bounded window", which was asserted in six places and built in none** |
| **The prompt is composed by the daemon from structured data**, with caller-supplied text shown only as marked, escaped, secondary content | absent. A prompt a caller can write is a prompt a caller can forge |

**The four channels, measured:**

| Channel | State |
|---|---|
| **Flag** | rig reserves exactly three flag names on a call - `json`, `timeout`, `args`. **The rest of the flag namespace is the PROGRAM's**: one flag is generated per declared schema property, so a program declaring a property named `confirm`, `yes`, `force` or `approve` gets that flag generated, completed and accepted, with no comment from rig. **The reservation list is the lever and no confirmation word is in it** |
| **Argument** | **wider than the flag channel and it defeats a flag reservation on its own.** `--args '<json>'` passes the whole argument object verbatim, schema-validated but flag-free. A confirmation expressed as an argument is satisfied in one call and no reserved-name check would see it |
| **Header** | **does not exist on either side.** The client call takes no metadata and the wire frame has no header or metadata map. **Nothing to close today**, and this clause exists to gate the day one is added rather than to repair anything |
| **Second call** | the id that would bind an approval is unset, above |

#### `confirms` is declared, rendered to a human, and consumed by nothing

**rig currently tells a user that a command confirms before acting, while rig
confirms nothing.** `Command.confirms` is a tristate the program declares about
itself. The kernel validates only that it is not unsaid; **no other reader
exists in the tree.** Generated `--help` prints *"it declares that it confirms
before acting"* and `rig apps list --json` carries `"confirms": true`.

**Demonstrated 2026-09-11, not read off the source:** the reference program
declares a `purge` command `destructive` with `confirms` yes, its generated help
says so, and invoked with no terminal it returns success and purges. **No
prompt, no refusal, no channel.**

**This is conflation #16 arriving on its own** - `confirmation` and
`authorization` conflated outright - and it is the sharper form of it, because a
conflation inside a document misleads a reader while **this one makes rig state
something false to a user.** The wording is the tell and it is doing real work:
*"it declares that it confirms"* is honest about the provenance and is read as a
guarantee.

**RULED 2026-09-11: `confirms` is an INPUT rig acts on, not a program's claim
about itself.** Delegated by Boris with the instruction to prefer robustness and
usefulness, and both point the same way here.

**A command declaring `confirms` is treated exactly as if a house rule had
matched it at `confirm`**, and it composes with the rules table through the
most-restrictive-wins rule already stated above. That single sentence is the
whole mechanism, and it is why this is the robust reading rather than the
ambitious one:

- **A declaration can only ever ADD a confirmation, never remove one.**
  Most-restrictive-wins makes `confirms: no` unable to weaken a rule, so the
  field cannot become a second lever on the authorization floor. **The
  weakening hole above does not reopen through this door**, and it would have
  if the composition rule were anything else.
- **It uses machinery that exists.** §13a already produces `confirm` as an
  action and already routes it. Nothing new is introduced at the boundary.
- **The alternative is strictly less useful.** Reading `confirms` as a claim
  means changing the rendering to disown it and leaving rig with a declared
  field nothing consumes - which is the state being repaired.
- **It fails closed.** A program that says it confirms gets a confirmation. The
  failure mode of getting this wrong is an extra prompt, not an unguarded
  destructive call.

**The rendering stops being false the moment this is built**, because *"it
declares that it confirms before acting"* becomes a description of an input rig
honours rather than a claim rig merely repeats.

**The last three are not connections, and that is why they are in the enum.** §14 says every
*connection* carries a principal, and a scheduled fire, a bus-triggered invocation and a
`rig://` URL are none of them - so without these three values the rule the owner most needs,
"the scheduler may not run destructive commands unattended", cannot be written at all. Each
mints a principal at the point of invocation: `schedule` from the schedule entry's owner,
`bus` from the rule's owner, and `url` from nothing, because a URL arrives from the browser.

**`url` is therefore denied above read-only and confirmed at it - which is two rules, and both
ship with the enum.**

```
[[rules]]
caller  = "url"
effects = "writes-files"
action  = "deny"

[[rules]]
caller  = "url"
effects = "read-only"
action  = "confirm"
```

A `rig://` scheme registered on the desktop is reachable from any web page, email or
chat message; §29's "the threat model is a mistake in our own code, not an adversary" was
written about hosted programs and does not cover an inbound handler that anything can address.
A destructive call arriving over `rig://` matches both rules - the first through the floor
above - and the precedence rule resolves it to `deny`. **It is written as two rules rather than
as prose because "read-only plus confirm" is not something the grammar can say in one**, and a
default that cannot be expressed in the grammar it ships beside is a default nobody can audit.
The URL surface itself is M13. **The enum ships at M1**, because it lives in the kernel's
invoker and the whole point of that placement is that a surface added in 2028 is covered by
rules written in 2026 - which only holds if the vocabulary can name it.

**A rule is a pair, and a wrapping verb splits it.** `effects` comes from the registry, per
command, declared by the program; `caller` comes from the invocation. A verb whose subject is
another command separates the two onto different objects - and one ships at M7:
`rig peers run --lease=NAME -- make deploy` (§16). Match on the outer verb and there is no
declared `effects`, because `rig peers run` has no registry entry. Match on the wrapped command
and the caller is no longer the principal that arrived. **Whichever half the invoker looks at it
loses the other, so the pair never completes and, against an empty default set, the call is
allowed.** The rule the owner most needs - "the scheduler may not run destructive commands
unattended" - is silently unwritable for exactly the callers it was written for.

**So the invoker matches on the effective pair, resolved once at the boundary.** `caller` is the
principal that arrived; `effects` is the union over every registered command the call will
actually run. A wrapping verb contributes no effects of its own and **cannot mask the effects it
wraps**. This is a property of the invoker, so it holds for a wrapping verb added in 2028 the
same way the caller enum does.

**A wrapped command rig cannot resolve counts as `destructive` for matching.** `make deploy` is
not a registered command and rig cannot know what it does. This does not change what happens
when no rule exists - an empty set still matches nothing - but it means a rule that names
`destructive` fires on the one case where the invoker cannot see what it is authorising, rather
than silently missing it. Unresolvable and harmless is a combination only the program can
declare, and it has not.

- **It lives in the kernel's invoker**, so no surface can forget it and a surface added in 2028
  is covered by rules written in 2026. One test covers every surface, present and future.
- **The default rule set is the two `url` rules and nothing else**, so nothing already working
  breaks the day it lands: no surface mints a `url` principal before M13, and every other
  caller starts unmatched. An unmatched call is allowed, which is what makes the table safe to
  land early - and is why the M1 demo writes a rule rather than relying on a default.
- **It costs a registered program nothing.** Both fields it matches on are already declared.
- `confirm` routes through the same `ask` primitive the peers service uses (§16), so the
  question reaches whoever is actually present - window, toast, terminal or phone.
- **Two kinds of ask, and until now the plan had one failure mode.** A **gating** ask fails
  *closed*: an elevation nobody answers is denied and recorded (§14), because the alternative
  is estate-wide action taken by default. A **disambiguating** ask fails to *none*: it parks,
  the way §9's `interactive-stream` run parks rather than fails, and the caller proceeds
  without the thing it asked about. "Which of these three continuations?" (§16) must not be
  *denied* when nobody answers - denying it strands a replacement that could have started
  cold. Every `ask` declares which kind it is, and **the default is gating**, because that is
  the half where guessing wrong is unsafe.
- Every decision is written to the audit log with **`origin`**, which is `rule` or
  `elevation`, and the rule id when there is one. An elevation is not triggered by a house
  rule - it is triggered by the call needing estate-wide authorisation (§14) - so an `origin`
  that can only name a rule has no value for the commonest `confirm` the system will produce,
  and the log cannot then answer the first question anyone asks of it. One more enum value
  now; a schema migration later.
- With that, "why was I asked" and "why was that allowed" are answerable from the record.

---
