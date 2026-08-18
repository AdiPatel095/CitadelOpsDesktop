# LLM Control Plane — Design Notes

> **Status: future feature, not started, not decided.** This records what the
> codebase already provides, what would have to be built, and the risks that are
> specific to putting a language model in front of the intent engine. It is not a
> committed plan and nothing here is scheduled.

Attach a language model to the app so the user can describe what they want in
natural language and have it adjust settings, and — later — take game actions.

---

## 1. The headline

**Most of this already exists.** The intent engine is, structurally, a tool-use
API that predates the decision to expose it as one.

| Tool-use needs | The app already has |
|---|---|
| A list of callable operations | `GET /api/v2/intents` — 111 registered definitions |
| Name + description per operation | `Intent.Definition.Name` / `.Description` |
| A permission model | `Intent.Definition.Effect` — read / write / launch / external |
| A way to invoke one | `POST /api/v2/intents/{name}` |
| Propose-without-executing | `Intent.Request.DryRun` |
| Guards that survive a bad caller | The full engine guard stack |

The work is a **translation layer over two endpoints that already exist**, not a
new capability layer. That is the single most important thing to understand
before estimating this.

### The intent surface today

`Server/API/Server.go:97-98`:

```
GET  /api/v2/intents        → server.handleIntentDefinitions
POST /api/v2/intents/{name} → server.handleIntentSubmit
```

`POST` is asynchronous by default: it answers `202` with the accepted receipt
and the runtime keeps executing the operation whether or not the caller stays
connected; follow `GET /api/v2/operations/{id}` or the `operation.changed`
stream, or pass `?wait=true` to block for the final receipt (the wait can be
abandoned without cancelling the operation; `POST /operations/{id}/cancel` is
the only client-side cancellation).

`Intent.Definition` (`Server/Intent/Types.go:139`):

```go
type Definition struct {
    Name             string          // "attack.launch", "config.update", ...
    Description      string          // one line, written for a human
    Effect           Effect          // read | write | launch | external
    ArgumentsExample json.RawMessage // example payload, not a schema
    // ... planner, read-set resolver (not serialised)
}
```

`Registry.Definitions()` (`Server/Intent/Registry.go:63`) returns all of them,
name-sorted. Enumeration is free.

### Registered intents by effect

| Effect | Count | What it covers |
|---|---:|---|
| `read` | 29 | refreshes, map queries, scans — no game-visible mutation |
| `write` | 56 | recruit, build, craft, equip, **and `config.update`** |
| `launch` | 20 | anything that commits troops |
| `external` | 6 | session start/stop, browser select, game-data refresh, app update |
| **Total** | **111** | |

---

## 2. Settings changes are one intent

The user's stated first goal — *let the LLM change settings* — does not need a
tool per feature. Every configuration write in the app goes through a single
registered intent (`Server/App/Application.go:507`):

```go
{
    Name: "config.update",
    Description: "Atomically update one versioned user-configuration section",
    Effect: Intent.EffectWrite,
    // Claims: "configuration:" + section
    // Step:   config.update → Configuration.UpdateConditional(
    //             section, value, expectedRevision, expectedValue)
}
```

Three properties matter here:

1. **Per-section claim.** `configuration:<section>` means two concurrent config
   writes to the same section serialise, and writes to different sections do
   not block each other.
2. **Optimistic concurrency.** `UpdateConditional` takes `ExpectedRevision` and
   `ExpectedValue`. An LLM working from a config snapshot it read a minute ago
   cannot silently clobber an edit the user made in the meantime — the update is
   rejected and has to be re-planned.
3. **Config changes already wake policies.** A policy declares `WakeSections()`;
   a config write clears its `nextCheck` so it re-evaluates immediately rather
   than waiting out its interval. So "turn Auto Khan's taunt threshold down"
   takes effect on the next coordinator pass, through the normal path, with no
   special handling.

**Phase one of this feature is one tool.** That is worth stating plainly because
it changes the shape of a first release: a useful settings assistant does not
require solving the 111-tool problem in §5.

---

## 3. `DryRun` is the propose-then-confirm primitive

`Server/Intent/Engine.go:347`:

```go
receipt.Plan = &plan
if firstPlan && request.DryRun {
    receipt.Status = StatusPlanned
    receipt.Phase  = EffectPhaseCompleted
    // ... no dispatch
    return receipt
}
```

The planner runs in full. The receipt comes back carrying the complete `Plan` —
`Claims`, `Resources`, `Steps`, `Admission`, and a human-readable `Summary`
(`"Queue 5 stacks of 40 tools definition 614 at DesertTown"`) — and nothing is
sent to the game.

That gives the trust boundary for free:

```
LLM emits tool call
      ↓
submit with DryRun: true
      ↓
engine plans; returns Plan.Summary + Steps
      ↓
user reads what the ENGINE decided, not what the MODEL said
      ↓
approve → resubmit without DryRun
```

The user is never asked to approve model prose. They approve a plan a pure
planner function built from live state. This is the same split as the rift
scheduler in [`RiftRaidCoordination.md`](RiftRaidCoordination.md): **the
proposer is advisory, the local guards are authoritative.**

---

## 4. Why a bad model cannot break an invariant

An intent submitted by an LLM traverses exactly the same stack as one submitted
by a policy — there is no shortcut path and no way to construct a raw `%xt%`
frame. Documented in full in
[`IntentEngineCommandGuards.md`](../IntentEngineCommandGuards.md); the relevant
properties:

- **Claims** serialise conflicting work regardless of who asked.
- **Admission classes** and **outbound lanes** still apply — an LLM cannot make
  attacks leave faster than the 4–6 s `LaneAttackLaunch` spacing.
- **In-plan guard actions** re-validate live state milliseconds before dispatch,
  so a plan built on a stale premise fails at the gate rather than acting.
- **Response barriers** (`wire` / `committed` / `wire-then-committed`) still
  gate step completion.
- **Protection Mode, wall-guard, troops-away** guards are inside the planners
  and the guard actions, not in the caller.

The realistic failure mode for a hallucinated or mistaken intent is **a rejected
plan or one wasted action** — not a violated invariant, not a duplicated attack,
not a corrupted config. That is a stronger starting position than most
applications adding model-driven control, and it is a direct consequence of the
architecture already being decision-in / plan-out.

The bounded-stale-replan cap added this cycle (`maximumStaleReplans = 3`,
`Server/Intent/Engine.go`) also matters here: a caller that keeps producing
plans invalidated by live state gets stopped rather than looping.

---

## 5. What does not exist

### 5.1 No JSON Schema for arguments

`ArgumentsExample` is an **example payload**, not a schema. Tool use needs
`input_schema`. Two options:

- **Hand-write schemas** per intent — 111 of them, accurate but a large one-time
  cost and an ongoing drift risk against the planners.
- **Derive from the argument structs** — each planner decodes its own argument
  type; generating schemas from those types keeps them honest by construction.
  Preferred, but requires the argument types to be nameable, which not every
  planner does today (several decode inline).

Whichever route, pair it with `strict: true` tool definitions so parameter
validation is guaranteed at the API rather than discovered when a planner's
decode fails.

### 5.2 111 tools will not fit sensibly in context

Loading every definition up front burns context and buries the relevant tools.
The intended mechanism is **tool search**: mark the intent tools
`defer_loading: true` and declare a tool-search tool; the model searches the set
and only matching schemas are loaded.

The reason this specifically matters here: tool search **appends** schemas
rather than swapping the tool list, so it does not invalidate the prompt cache.
The stable prefix for this feature is large — see §5.3 — and worth keeping
cached.

### 5.3 No state-feeding strategy

The model needs to know what the account looks like. The naive approach —
dumping `GET /api/v2/state` — is the **exact path §3.1 of the architecture
review identifies as the worst performance problem in the app** (`Snapshot()`
deep clone, ~78,000× more expensive than `ReadOnlyView()`). Do not build the
feature on it.

Better shape:

| Layer | Content | Volatility |
|---|---|---|
| System prompt | `Docs/Features/*` — every feature, its loop, its guards | stable, cacheable |
| Tools | the 29 read-tier intents | pulled on demand |
| Per-turn | the user's question | volatile, after the cache breakpoint |

The `Docs/Features/` set written this cycle (16 files, 1,384 lines) is already
structured as an explanation of what each feature does and what stops it — it
was written for humans but reads as an LLM knowledge base with no changes.

### 5.4 No credential handling

An API key is a user-supplied secret. The app's config is versioned, exportable
(`GET /api/v2/config/export`) and importable. **A key stored in a normal config
section would leak through the export.** Needs either a section excluded from
export or storage outside the config store entirely.

---

## 6. Where it runs

Two placements, and §6.1 of the architecture review decides between them.

| | In the Go binary | In the React client |
|---|---|---|
| Works headless | ✅ | ❌ |
| Works for a hosted instance | ✅ | ❌ |
| Key lives in one place | ✅ | ❌ (per browser) |
| Needs a Go SDK dependency | ✅ | — |

**Recommend in-process, server-side.** The headless binary is locked in (§6.1);
a control plane that only works when a UI is attached contradicts it. That means
the `anthropic-sdk-go` client and the tool loop live in the Go server, with the
client rendering the conversation over the existing event stream.

Implementation defaults if and when this is built: model `claude-opus-5`,
adaptive thinking, streaming (the tool loop involves long turns), and the SDK
tool-runner rather than a hand-written loop — its per-turn hooks are where the
`DryRun` approval gate naturally sits.

---

## 7. Risks

### 7.1 Prompt injection — the one genuinely new risk

Everything the app does today is machine-driven over a binary-ish protocol. An
LLM reading game state introduces **attacker-controlled text into the model's
context** for the first time:

- alliance and global chat
- player names, alliance names, castle names
- mail
- battle report text

A hostile player can choose those strings. A castle named to look like an
instruction is a live attack path the moment the model holds write or launch
tools.

Mitigations, in order of importance:

1. **Launch tier never auto-executes.** Not configurable, not a toggle.
2. **Treat every game-sourced string as data.** Fence it in the prompt and say
   so explicitly.
3. **`DryRun` + explicit approval for anything above read.**
4. Consider withholding chat and mail from context entirely in a first release —
   they are the highest-injection, lowest-value inputs.

### 7.2 Permission tiers

`Effect` is already the right axis, and the user's own phasing maps onto it:

| Tier | Count | Proposed default |
|---|---:|---|
| `read` | 29 | auto-allow |
| `write` — `config.update` only | 1 | allow, or confirm per section |
| `write` — everything else | 55 | confirm |
| `launch` | 20 | confirm, always, no auto option |
| `external` | 6 | confirm (session/browser control) |

### 7.3 Contention with policies and other instances

An LLM acting is a second actor competing for the same claims and lanes as 19
policy lanes. It must go through the coordinator and engine like everything
else — never around them. Two specific interactions:

- **Actor identity is an exact string match** (§4.4 of the architecture review).
  An LLM actor needs an ID that does not collide with a feature's, or its
  operations will be mistaken for a policy's.
- If a central rift scheduler is ever running (§6.3), the LLM is a *third*
  proposer. The precedence between "scheduler says launch at T" and "user asked
  the model to do something now" is undefined and would need deciding.

### 7.4 Cost and expectation

Each turn with a large cached prefix plus tool results is not free, and the user
supplies the key. Worth surfacing token spend in the UI rather than letting it
be invisible.

---

## 8. Effort read

| Piece | Size | Note |
|---|---|---|
| Settings-only assistant (`config.update`) | **Small** | One tool. `DryRun` gate. Reuses everything. |
| Argument schemas for all 111 intents | **Medium** | Codegen from argument types is the durable route |
| Tool search + context strategy | **Small-medium** | Mechanism is standard; the tuning is the work |
| Read-tier tools | **Small** | 29 intents, no approval flow needed |
| Write/launch tiers + approval UI | **Medium** | The UI is most of it |
| Credential handling | **Small** | But must be settled before any release |
| Injection hardening | **Ongoing** | Not a one-time task |

The honest summary: **a settings assistant is a small feature because the app
was built decision-in / plan-out.** Extending it to game actions is mostly
schema generation and approval UI, not engine work. The part that does not get
easier with time is §7.1.

---

## 9. Open questions

**How much should the model see?** Full config is large and mostly irrelevant to
any given question. Read-tier tools let it pull what it needs, but each pull is
a round trip. Where is the line?

**Is `config.update`-only worth shipping alone?** It is a genuinely useful
feature (the app has a lot of settings across 16 features) and it is small. Does
that justify shipping before the tool-surface work?

**Precedence between proposers.** If the rift scheduler and the LLM both want
the attack lane, who wins? Today the answer is "whoever claims first", which is
probably wrong.

**Does the LLM get to enable/disable features?** `automation.enabled` is a
config section, so `config.update` reaches it. Turning Auto Bird off is a
one-line config write with real consequences. Should some sections be
LLM-read-only?
