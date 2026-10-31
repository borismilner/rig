## 57. One question to the owner

The platform roadmap, section 9 (2026-10-12), calls for one ask
primitive with `{kind, urgency, refs, options, ttl}` and one policy
engine. This section specifies the rig half. An ask is a question
requiring Boris's decision. It is distinct from a notice and from
Marvin's `/ask`, where Boris asks a model a question.

### Contract

- `ask.open` validates and files one question, returning its id at once.
  The caller provides a stable idempotency key, kind, urgency, short
  title, optional body, references, up to three options, whether text
  is accepted, a TTL and a failure mode. A cloud caller never waits
  for a person.
- The default question modes are section 13's **gating** (expires denied)
  and **disambiguating** (expires with no answer). Gating is the
  default. Neither mode turns silence into approval. Section 16's explicit
  **countdown** mode shares this lifecycle: presentation arms a grace,
  Stop/dismissal stops it, and durable elapsed grace returns `proceed`.
  `ask.presented` belongs to the interactive renderer; missing presentation
  or suppressed delivery denies it.
- `ask.get` and `ask.list` read current state, including what channel
  delivered it and why. `ask.answer` accepts one offered option, text
  when offered, or dismissal. The first valid answer wins. `ask.withdraw`
  lets the creator take back an open question. Expiry and withdrawal
  refuse late answers.
- The question and each transition are durable in rig's record. After
  a restart, pending questions are reconstructed from the record and
  delivered again only when still open. Idempotency prevents a retry
  from making two cards. The answer is attributed to the channel and
  authenticated person, not to a field supplied by a caller.

### Policy and delivery

- Rig resolves one policy per kind: always ask, never ask, or ordinary;
  quiet hours; a rate limit; and an ordered channel choice for each
  urgency. A never rule answers gating as denied and disambiguating
  as none. An always rule bypasses quiet hours and pacing, never a
  security denial. No rule approves an OTP or an elevated action.
- A gating question bypasses Do Not Disturb. Urgent questions may use
  the phone while the laptop is off; a cloud run only opens the ask
  and finishes. A channel may report delivery failure, which rig keeps
  visible and retries through the next allowed channel. Delivery is
  not an answer.
- Rig's current toast and reply path becomes the first renderer of an
  ask. The phone's notification and Tulip, then buddies' asks, use the
  same contract. AgentBox cards are adapted last. Telegram may carry
  sender and title only; it is not an answer channel until authenticated
  answers are implemented.
- References identify material kept by its owning program. The ask
  record never carries an OTP candidate, password, or other secret.
  The phone's OTP broker holds a candidate locally and releases it to
  the one requesting client only after Boris's tap. A denied or expired
  request returns no candidate.

### Rollout gate

The new rigd is exercised on a scratch estate with an unanswered TTL,
one winning answer, a late answer refusal, a restart while pending,
quiet hours and rate rules, and a failed delivery. It is not deployed
to the laptop or cloud before the review hold on rigd is lifted.
