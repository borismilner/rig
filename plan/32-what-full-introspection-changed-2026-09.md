## 32. What full introspection changed, 2026-09-10

A requirement arrived after the attack and after the advocate pass: **rig must be fully
introspectable to the agents Boris runs.** It is not a new feature - §3 already called
introspection a maximal and §9 already called an agent a first-class user - but it decides a
contradiction the attack created and named without resolving.

**The contradiction.** The attack's highest-ranked finding made the operator a credential
rather than a uid, correctly. §14 then made every estate-wide view an operator surface -
seven of them - and observed in its own text that "one of which §9 tells an agent to read
first". §9 kept telling agents to read the capability map; §14 had just made sure they could
not. Both sections were individually right and jointly wrong, which is precisely the failure
mode of ten seats each attacking one surface.

| Was | Is |
|---|---|
| One operator credential, minted to a path only the launching process gets | **Two grants and no credential.** `introspect` is **decided at connect time** from whether the connection registered as a program; `operate` is an authorisation decision per call, through `confirm` for anyone present and a named `house rules` entry for anything unattended. Nothing is minted, distributed, inherited or persisted. Two drafts that did distribute something - a 0600 file, then `RIG_INTROSPECT` in the environment - are recorded in §14 with how each broke, and §33 has the sweep that killed the second |
| Estate-wide views closed to every agent | Open to any agent Boris runs, completely - the full map, any client's history, every trace, every config resolution |
| §3's isolation test: "two clients cannot see each other" | Three runs. Ungranted A sees nothing of B; A holding `introspect` sees **all** of B and a missing row is a failure; and no run at any sampling rate surfaces a `sensitive` value |
| "Looking is an event", one audit entry per read | Coalesced per principal, per view, per minute, with a count. Presenting a grant stays uncoalesced |

**The argument that makes it safe is one the plan already owned.** §15 does not filter secrets
out of the history; it never writes them. So the worst an introspecting agent can read is
everything Boris could read himself. **Redaction is the precondition for complete
introspection**, and the two features are complements rather than a trade.

**What this deliberately does not do.** It does not widen what an agent may *run* - `house
rules` (§13a) is untouched and a destructive command still needs its grant. It does not make
programs introspectors: a connection that registered as a program is scoped for its life, and
there is no variable, file or token for a program to acquire instead.

**Found by the blind-spot sweep, an hour after the first draft, and then again an hour after
the second.** Round 1 caught two defects while the change was still uncommitted: the grant
delivered as a uid-readable file, and `operate` distributed to a process no human surface
descends from. **Both are the same mistake** - reasoning about the grant from the reader it was
written for, and not from the other four kinds of caller.

The second draft made that same mistake four more times, and round 2 is what caught it: the
environment reaches a shell's whole descendant tree and goes stale on restart (D1, D2, D3), a
hosted program is the one class no environment scrub can cover (D4), `make deploy` cannot read
the launcher's file (D9), and §12 never carried the Do Not Disturb exception §14 relied on
(D13). **The durable lesson is narrower than "run a sweep":** anything that adds a principal,
a grant or a scope gets walked against the full caller table, which is why §14 now has one.
The record is
`logbook/projects/rig/agent-work/attack-2026-09-10-s9-blindspot-sweep/FINDINGS.md`.

**And one thing it costs, written down rather than argued away.** The session boundary in
§14's table was previously enforced for every client; it is now enforced against accident and
against a merely buggy client, and a program that opens a second connection and declines to
register on it reads everything. That is the floor of same-uid isolation on Linux and no
arrangement of tokens beats it. Three things remain genuinely enforced against a hostile local
process: the uid boundary, `operate`, and redaction. **The first draft of this change put the
grant in a 0600 file, which would have made the session boundary a comment** - it is recorded
here because the next person to simplify the delivery will reach for exactly that file.

---
