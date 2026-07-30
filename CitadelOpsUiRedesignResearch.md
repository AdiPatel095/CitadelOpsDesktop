# CitadelOps UI, Product, Brand, and Web Ecosystem Redesign Research

**Research and redesign brief**  
**Prepared:** July 14, 2026  
**Scope:** Public website, account and licensing portal, desktop operations console, automation
authoring and supervision, activity and diagnostics, AI-assisted interaction, brand, icon, design
system, accessibility, and performance  
**Status:** Research synthesis and directional product specification. It is not a final visual
design, legal clearance opinion, engineering backlog, estimate, or release commitment. The roadmap
is directional sequencing only.

---

## Executive overview

CitadelOps should become a calm, exception-first command center that makes automation legible and
controllable. The interface should lead with system safety, freshness, actionable exceptions,
current work, and what happens next; raw evidence should remain one deliberate drill-down away.

The existing application already contains the right trust primitives—deterministic intents, dry
runs, state revisions, resource claims, operation status, receipts, cancellation, readable
telemetry, and raw evidence. The redesign should expose those primitives rather than inventing a
decorative dashboard layer over them.

The larger product should be one coherent ecosystem with three distinct surfaces: an expressive
public website, a quiet account/licensing portal, and a productive desktop operations console. A
constrained AI copilot should operate through the same typed intent and receipt model as every other
caller.

Before the public identity is finalized, the product name needs professional clearance. Preliminary
web research found an operating South Florida IT company using “CitadelOps” in adjacent security,
cloud, monitoring, and automation services, making that a P0 business and brand gate.

### Reading map

- [Command-center model](#the-command-center-model)
- [Automation UX](#automation-ux-from-switches-to-a-control-tower)
- [AI-assisted interaction](#ai-assisted-interaction)
- [Alerts and notifications](#alerts-notifications-and-messaging)
- [Activity, traces, and raw logs](#activity-execution-traces-and-raw-logs)
- [Strategic decisions and brand gate](#strategic-decisions-and-brand-gate)
- [Research method](#how-this-research-was-conducted)
- [Complete UI-design problem space](#the-complete-ui-design-problem-space)
- [Product and repository audit](#product-and-repository-audit)
- [Experience principles](#experience-principles)
- [Users, contexts, and risk](#users-contexts-and-risk-model)
- [Target ecosystem and information architecture](#target-product-ecosystem-and-information-architecture)
- [Public website](#public-website-specification)
- [Account, authentication, licensing, and billing](#account-authentication-licensing-and-billing)
- [Onboarding and help](#onboarding-empty-states-and-help)
- [Color theory and color system](#color-theory-and-the-proposed-color-system)
- [Brand and identity](#brand-strategy-and-identity)
- [Typography](#typography)
- [Layout and responsive behavior](#layout-density-and-responsive-behavior)
- [Motion](#motion)
- [Iconography](#iconography)
- [Data visualization](#data-visualization)
- [Accessibility](#accessibility-specification)
- [Performance and resilience](#performance-and-resilience-specification)
- [Design system](#design-system-architecture)
- [Localization](#localization-and-internationalization)
- [Screen blueprints](#recommended-screen-set-and-blueprints)
- [Roadmap](#implementation-roadmap)
- [Validation](#validation-program)
- [Source library](#source-library)

### Evidence labels used in this report

- **Standard / known constraint:** normative requirement, current official guidance, or observed
  product fact.
- **Research-supported pattern:** established human-factors or mature-product practice that still
  requires adaptation to CitadelOps.
- **CitadelOps hypothesis:** directional choice such as Command Violet, a timing guardrail, palette
  proportion, or concept mark that must be validated before it becomes a product standard.

In this report, **operational safety** means preventing unintended or duplicate game commands,
premium/scarce-resource loss, action from stale state, hidden scope, unrecoverable retries, account
disruption, and loss of user control. It does not mean physical safety or safety-critical
certification.

---

## The command-center model

The desktop home screen is not a report. Its job is to establish situation awareness and direct
attention.

The January 2026 release of the U.S. Nuclear Regulatory Commission's
[NUREG-0700 Revision 4](https://www.nrc.gov/reading-rm/doc-collections/nuregs/staff/sr0700/index)
provides a useful evaluation taxonomy for complex control rooms: information display, interaction,
alarms, soft controls, procedures, automation, communication, workstations, degraded conditions,
maintainability, and integration. CitadelOps should not imitate nuclear-control visuals, but it
benefits from the same discipline of making modes, alarms, procedures, and degraded states
unambiguous.

[Google SRE monitoring guidance](https://sre.google/workbook/monitoring/) separates alerting,
investigation, visualization, and long-term analysis.
[Grafana's dashboard guidance](https://grafana.com/docs/grafana/latest/visualizations/dashboards/build-dashboards/best-practices/)
similarly recommends that a dashboard answer a question, tell a story from overview to detail,
minimize cognitive load, and refresh no faster than the data changes.

### Overview questions, in order

1. Is CitadelOps connected, licensed, current, and operating safely?
2. Which castle, automation, or shared resource needs attention?
3. What is automation doing now?
4. What will it do next, and when?
5. What recently succeeded or changed materially?
6. Where can I inspect and recover an abnormal outcome?

### Recommended Overview structure

1. **Quiet global health strip**
   - Desktop service.
   - Game connection and login.
   - State freshness.
   - License/entitlement exception only when relevant.
   - Update exception only when relevant.
   - Persistent Pause all automations action when any automation is active.
2. **Attention queue**
   - Only actionable, durable exceptions.
   - Grouped by common cause.
   - Ordered by consequence and time sensitivity.
3. **Empire/castle summary**
   - Current meaningful mode.
   - Active automation/current step.
   - Next action or deadline.
   - Resource pressure.
   - Freshness.
4. **Automation health**
   - Running, waiting, blocked, failed, paused.
   - Resource/contention summary.
5. **Upcoming operations**
   - Queue and timeline rather than generic charts.
6. **Recent significant outcomes**
   - Human-readable receipts, not every event.
7. **Optional trends**
   - Only where they support a decision or reveal degradation.

### What should not appear on Overview by default

- Raw protocol frames.
- Every WebSocket event.
- Vanity metrics without a decision.
- A grid of identical feature cards.
- A giant “AI chat” entry point.
- Billing upsells.
- Developer tools.
- Healthy connection details that are already summarized.
- Repeated green “all good” badges.

### Attention queue object

Every attention item should contain:

| Field                       | Purpose                                                 |
| --------------------------- | ------------------------------------------------------- |
| Plain-language title        | What requires attention                                 |
| Consequence                 | Why it matters now                                      |
| Scope                       | Empire, castle, automation, execution, resource         |
| First and latest occurrence | Duration and recurrence                                 |
| Freshness                   | Whether the conclusion is based on live or cached state |
| Cause grouping              | Prevents cascades from becoming an alarm flood          |
| Recommended action          | The best next step                                      |
| Owner/actor                 | User, automation, AI, Citadel, or game provider         |
| Recovery state              | Open, acknowledged, snoozed, recovering, resolved       |
| Evidence link               | Filtered operation/activity/diagnostics                 |

Acknowledgment is never presented as resolution. Snooze records who, why, and until when. Recovery
visibly clears or downgrades an item. Flapping conditions use pending and recovery thresholds rather
than repeatedly firing.

### Orthogonal operational status

Do not compress all status into one red/yellow/green dot. Track these dimensions independently:

| Dimension    | Values                                                                          |
| ------------ | ------------------------------------------------------------------------------- |
| Lifecycle    | Draft, Waiting, Running, Paused, Blocked, Retrying, Failed, Completed, Disabled |
| Attention    | None, Information, Warning, Critical                                            |
| Freshness    | Live, Delayed, Stale, Offline, Unknown                                          |
| Scope        | Global, account, castle, automation, execution, step                            |
| Actor        | User, automation, AI, Citadel system, game/upstream                             |
| Availability | Available, partially available, unavailable, unsupported                        |

An automation can be **Running + Warning + Stale** at the same time. A single orange badge cannot
communicate this correctly.

[Carbon's status pattern](https://carbondesignsystem.com/patterns/status-indicator-pattern/)
recommends avoiding indicators for states that do not need attention and combining color,
symbol/shape, and text. CitadelOps should go further by exposing reason, time, and consequence where
status drives a decision.

### Global connection model

Model each layer separately:

```text
Account entitlement
        ↓
Citadel cloud / account API (if used)
        ↓
Local desktop process
        ↓
Managed browser lifecycle
        ↓
Game WebSocket
        ↓
Game login/session
        ↓
Fresh normalized game state
```

The summary might say **Connected · live 8s ago** in healthy state. When degraded, it expands:

```text
Game connection interrupted
Last confirmed state 2m 14s ago
Desktop service and account are healthy
Retrying automatically in 18s
Unsafe write actions are paused
[View connection details] [Retry now]
```

An online browser flag is insufficient evidence that the required service is reachable;
[MDN notes this limitation](https://developer.mozilla.org/en-US/docs/Web/API/WorkerNavigator/onLine).
Health must be determined at the actual service boundaries.

---

## Automation UX: from switches to a control tower

### Mental model

Separate five objects:

1. **Definition** — the saved automation recipe and policy.
2. **Draft** — uncommitted edits.
3. **Live version** — the deployed definition currently eligible to run.
4. **Execution** — one run with its own state, input, steps, and outcome.
5. **Input snapshot** — the GameState revision/freshness used to decide and act.

Without this separation, users cannot tell whether editing a setting changes the running system
immediately, whether a history item used old logic, or whether retrying will repeat work.

### Automation control-center row

Each feature row should expose:

- Name and concise purpose.
- Lifecycle label with icon and color.
- Plain-language status detail.
- Scope: empire, all castles, selected castles, or one castle.
- Current step, if running.
- Next expected action or check and time.
- Last significant outcome and time.
- Freshness of the input state.
- Dependencies: schedule, preset, commander, slots, currency, reserves, browser/session.
- Draft/live mismatch.
- Error or blocked reason.
- Direct **Pause**, **Inspect**, and **Settings** paths.
- Link to last operation/activity.

Example:

```text
Auto TCI                                      Waiting
All castles · 4 enabled
Next: upgrade Bakery at GhostTown when slot 2 is free · about 18m
Blocked at Winter Keep: no matching construction item
Last action: Workshop upgraded to level 7 · 3m ago
[Pause] [Inspect 1 issue] [Settings]
```

### Canonical state vocabulary

| State       | Meaning                                         | User question answered           | Primary controls        |
| ----------- | ----------------------------------------------- | -------------------------------- | ----------------------- |
| Draft       | Defined but not active                          | What must I finish?              | Edit, validate, discard |
| Disabled    | Not eligible to schedule                        | Why is nothing happening?        | Enable                  |
| Waiting     | Eligible; conditions not currently met          | What is it waiting for?          | Inspect, pause          |
| Scheduled   | Next evaluation/action has a known time         | When will it run?                | Inspect, reschedule     |
| Running     | An execution is active                          | What step is happening?          | Inspect, stop safely    |
| Paused      | No new work will start                          | What remains in progress?        | Resume, inspect         |
| Blocked     | Cannot progress without an external/user change | What must be fixed?              | Resolve, inspect        |
| Retrying    | Recoverable failure with bounded retry          | When and why is retrying?        | Cancel retry, inspect   |
| Failed      | Execution ended unsuccessfully                  | What happened and is retry safe? | Recover, inspect        |
| Completed   | Run achieved its goal                           | What changed?                    | View receipt            |
| Unavailable | Feature cannot operate in current environment   | What dependency is missing?      | Resolve, learn more     |

Do not use red for Disabled. Disabled is neutral. Reserve danger color for harmful, failed, or
destructive conditions.

### Definition editor

Common automations should use a guided sequence:

1. **When** — trigger, cadence, or eligibility.
2. **If** — optional conditions.
3. **Then** — intended actions.
4. **Scope and resources** — castles, presets, commanders, reserves, budgets.
5. **Limits and stop conditions** — maximum runs/spend, cooldown, time window.
6. **Failure behavior** — retry, pause, skip, escalate, or fallback.
7. **Review and dry run** — exact current effect before deploy.

Simple recipes should not require a graph editor. Branching, retries, and expert graphs can appear
progressively.

[Home Assistant's trigger-condition-action model](https://www.home-assistant.io/docs/automation/editor/)
is approachable for simple rules.
[AWS Workflow Studio](https://docs.aws.amazon.com/step-functions/latest/dg/workflow-studio.html)
demonstrates synchronized visual/code representations, validation, reusable patterns, contextual
inspectors, and state-level testing. CitadelOps should adopt the useful concepts without copying
either product's visual language.

### Authoring requirements

- Validate before deploy.
- Show unsaved and undeployed changes.
- Name the live version and revision.
- Preview exact castles and resources affected.
- Dry-run against current state without sending commands.
- Test a condition, one step, or a complete sequence.
- Explain trigger-event versus current-state-condition behavior.
- Support templates, duplicate, notes, import/export, and stable IDs.
- Preserve a version with every execution so history remains explainable.
- Warn when the dry-run input becomes stale before confirmation.
- Show conflicting claims and schedules before activation.

### Dry-run review

```text
Auto TCI — proposed version 8

Based on GameState revision 18294 · live 6s ago
Scope: GhostTown, Winter Keep, Stonewatch

Expected actions in the next 60 minutes
1. GhostTown: equip TCI 418 in Bakery slot 2
2. Winter Keep: wait; construction slot occupied for 23m
3. Stonewatch: no action; reserve threshold would be crossed

Resource effect
Silver: up to 12,500
Premium currency: prohibited
Shared slots: 1 construction claim

Stop conditions
Pause after 2 consecutive failures
Never retry a confirmed purchase

[Deploy version 8] [Edit] [Save draft]
```

### Execution trace

Run history is a first-class product, not an afterthought.
[Home Assistant traces](https://www.home-assistant.io/docs/automation/troubleshooting/) show the
executed path, condition results, skipped branches, timeline, and related activity without forcing
users into raw logs.
[AWS execution details](https://docs.aws.amazon.com/step-functions/latest/dg/concepts-view-execution-details.html)
emphasize the precise failed state and cause.

An execution detail should contain:

- Definition and version.
- Trigger and input GameState revision/freshness.
- Scope and actor.
- Timeline of steps.
- Conditions with pass/fail/skip/unknown/invalid states.
- Commands attempted and game acknowledgment/confirmation.
- Resource claims and changes.
- Duration and time spent waiting.
- Retry count, backoff, and next retry.
- Exact failure and plain-language cause.
- Completed versus incomplete work.
- Safe recovery choices.
- Correlation/operation ID behind technical disclosure.

### Recovery and redrive

Do not offer a generic “Run again” for workflows containing non-idempotent purchases, upgrades, or
launches.
[AWS redrive](https://docs.aws.amazon.com/step-functions/latest/dg/redrive-executions.html)
demonstrates resuming from an unsuccessful step while preserving completed work.

Recovery choices should be generated from known execution state:

- Retry only the failed safe step.
- Resume after a dependency is fixed.
- Skip an optional item.
- Re-evaluate against fresh state.
- Roll back a reversible local setting.
- Create a new execution from a revised definition.
- Stop and require manual review.

For each choice, state what will and will not be repeated.

### Failure-control requirements

- Timeout.
- Maximum retries.
- Exponential backoff with visible next time.
- Catch/fallback behavior.
- Cooldown.
- Concurrency policy.
- Rate limit.
- Resource budget.
- Stop conditions.
- Circuit breaker for repeated upstream failure.
- Idempotency or explicit non-idempotency classification.
- Operation receipt and confirmation level.

### Shared-resource allocation

Commander assignments, attack slots, currencies, troops, tools, and time skips are shared
operational resources. They deserve a cross-automation allocation view.

Recommended views:

- **Now:** current claims, conflicts, and idle capacity.
- **Upcoming:** schedule/timeline of expected claims.
- **Policy:** priorities, reservations, and preemption rules.
- **Conflict inspector:** which work is blocked and why.

Avoid allowing each automation settings modal to hide an independent commander or budget rule that
conflicts elsewhere.

### Global stop semantics

| Control              | Exact effect                                                        |
| -------------------- | ------------------------------------------------------------------- |
| Pause new work       | No new executions start; current safe work continues                |
| Stop after safe step | Current operation reaches its next defined safe boundary            |
| Stop now             | Cancel what can safely be canceled; explain what cannot be reversed |
| Disable automation   | Prevent future scheduling until explicitly enabled                  |
| Disconnect game      | End connectivity; not a substitute for an automation stop policy    |

The persistent shell control should normally be **Pause all automations**. More forceful options can
live in its expanded menu with explicit consequences.

---

## AI-assisted interaction

### Appropriate role

The AI should primarily:

- Summarize current state and attention items.
- Explain why an automation is waiting, blocked, or failed.
- Recommend settings with assumptions and tradeoffs.
- Draft an automation from a natural-language goal.
- Convert a broad request into typed intents and a readable plan.
- Compare options and resource impact.
- Help filter activity or diagnostics.
- Produce a support-ready incident summary.

The AI should not:

- Silently execute high-impact game actions.
- Bypass the intent engine.
- Hide uncertainty or stale input.
- Invent unsupported causes.
- Use raw websocket commands as its normal action surface.
- Request or expose credentials in conversation.
- Present a recommendation as guaranteed.

[Microsoft's 18 human-AI interaction guidelines](https://www.microsoft.com/en-us/research/publication/guidelines-for-human-ai-interaction/)
emphasize capability/limit communication, efficient invocation and dismissal, correction,
explanation, and granular feedback. The
[NIST AI Risk Management Framework](https://airc.nist.gov/airmf-resources/airmf/) emphasizes
explicit oversight, documentation, and accountability. Those principles fit the existing CitadelOps
deterministic intent model well.

### Permission ladder

1. Observe only.
2. Recommend.
3. Draft an automation or plan.
4. Execute reversible/low-risk commands after scoped approval.
5. Execute within explicit standing budgets and action classes.
6. Prohibited classes that always require direct human action.

Permissions should be visible, revocable, scoped, and time-bounded. “Allow AI” is too broad.

### AI proposal card

Every proposed action includes:

- What the AI observed, including source and freshness.
- Why it recommends the action.
- Affected castle/account/resources.
- Effect classification.
- Expected cost and outcome.
- Claims or work that could be blocked.
- Preconditions and expected state revision.
- Readable dry-run plan.
- Uncertainty or missing information.
- Reversible and irreversible consequences.
- Confirmation requirement.
- Stop/override mechanism.

Example:

```text
Suggestion: delay Auto Recruit at Winter Keep for 42 minutes

Why: food is projected to cross the configured reserve before the next supply movement.
Based on: live state 7s ago and 3 inbound movements.
Effect: changes one schedule; sends no game command now.
Uncertainty: one movement has an unconfirmed arrival estimate.

[Preview schedule change] [Dismiss] [Always ignore this condition]
```

### Execution and audit

- The AI submits the same typed intent as any other caller.
- A dry-run is shown for material writes.
- The global automation pause and resource policies apply.
- Operation progress appears outside the chat surface.
- Receipts identify AI as proposer and the human/policy as approver.
- Feedback can correct the recommendation without granting broader authority.
- Conversation text is not the only audit record.

---

## Alerts, notifications, and messaging

### Core rule

If the user cannot take meaningful action, the event is not an alarm. Put it in activity history or
a dashboard summary.

The
[ISA-18 standards family](https://www.isa.org/standards-and-publications/isa-standards/isa-18-series-of-standards)
treats alarms as a lifecycle of philosophy, classification, prioritization, rationalization,
implementation, monitoring, and optimization. CitadelOps should adopt the discipline, not industrial
styling.

### Message taxonomy

| Pattern               | Use                                          | Persistence                           |
| --------------------- | -------------------------------------------- | ------------------------------------- |
| Inline validation     | Problem with current field/action            | Until fixed                           |
| Toast                 | Brief low-risk outcome, ideally reversible   | Timed, never for critical state       |
| Page banner           | Persistent condition affecting current scope | Until resolved/dismissed where safe   |
| Site/app alert        | Broad critical incident                      | Persistent and rare                   |
| Attention queue       | Durable actionable operational item          | Until resolved/explicitly managed     |
| Activity history      | Human-readable outcome record                | Retained per policy                   |
| Execution trace       | Step-by-step automation evidence             | Retained per operation policy         |
| Raw log               | Expert technical evidence                    | Bounded/rotated                       |
| External notification | Configured urgent event                      | User-controlled channel and threshold |

[Carbon's notification pattern](https://carbondesignsystem.com/patterns/notification-pattern/)
advises relevant, timely, informative, scoped, minimally disruptive messaging.
[USWDS alert guidance](https://designsystem.digital.gov/components/alert/) emphasizes human language
and a useful next step.

### Priority rubric

- **Critical:** harmful or irreversible action may occur, resource loss is imminent, or the system
  cannot continue safely. Never auto-dismiss.
- **High:** a running automation is blocked or repeatedly failing with a near-term consequence.
- **Medium:** degraded behavior or configuration needs attention but not immediate interruption.
- **Information:** routine completion or expected activity; history only by default.

### Quality requirements

- State trigger and consequence.
- Identify scope and owner.
- Provide the most useful next action.
- Group cascading symptoms under one cause.
- Do not inflate badge counts with routine history.
- Keep critical items visible until resolution.
- Distinguish acknowledgment, snooze, recovery, and resolution.
- Suppress or summarize repetitive noise.
- Link to a prefiltered operation or diagnostic context.
- Record who changed notification policy.

[Grafana's alerting guidance](https://grafana.com/docs/grafana/latest/alerting/guides/best-practices/)
recommends trigger, impact, ownership, action context, and links; if there is no action, a dashboard
is more appropriate.

### Error message formula

```text
[What happened]
[Affected scope and consequence]
[What remains safe or usable]
[Best next action]
[Optional technical details]
```

Example:

```text
Upgrade was not sent
Auto TCI paused at GhostTown because its source state became stale.
Other castles remain active. No currency was spent.
Reconnect and review the refreshed plan before retrying.
[Reconnect] [View execution] [Technical details]
```

Do not say “Something went wrong” when the system knows scope and next action. Do not invent a cause
when it does not.

---

## Activity, execution traces, and raw logs

### Five information layers

1. **Current state** — what is true now.
2. **Attention item** — what needs action.
3. **Activity event** — what materially happened.
4. **Execution trace** — how an operation evaluated and progressed.
5. **Raw structured log** — technical evidence.

The Overview might say:

> Auto TCI could not upgrade Workshop because the required item is unavailable.

**View execution** opens the evaluated path. **Technical details** opens diagnostics already
filtered to the castle, feature, operation, time range, and severity.

### Human-readable activity model

Each event should capture:

- Timestamp and observed timestamp.
- Actor.
- Action.
- Scope.
- Outcome.
- Reason or trigger.
- Resource effect.
- Operation/correlation ID.
- Definition and state revision where relevant.
- Links to affected entities.

The [OpenTelemetry Logs Data Model](https://opentelemetry.io/docs/specs/otel/logs/data-model/)
provides a useful structured foundation: timestamps, severity, body, resource, attributes, event
name, and trace/span correlation.

### Activity examples

```text
10:42:18  Auto TCI upgraded Workshop to level 7 at GhostTown
           Spent 12,500 silver · confirmed by game · Operation 8F2A

10:39:06  Auto Recruit is waiting at Winter Keep
           Food reserve would fall below 18,000 · next check in 4m

10:31:44  You paused all automations
           1 active operation will stop after its current safe step
```

### Diagnostics placement

Keep a remembered, resizable right or bottom utility dock and a dedicated Diagnostics route. The
dock can be opened by shortcut or from an operation. The closed affordance should show only unseen
warning/error count, not total log volume.

Default tab order:

1. Activity.
2. Execution details.
3. Diagnostics.
4. Raw frame.

### Log explorer requirements

- Filter by castle, account, module, actor, operation ID, severity, channel, opcode, and time.
- Full-text search and structured field filters.
- Absolute and relative timestamps with visible timezone.
- Expandable fields and formatted JSON only on demand.
- Surrounding context before and after a selected record.
- Pause/resume live tail.
- Preserve scroll when the user leaves the tail.
- Show “12 new events — Jump to latest” rather than snapping.
- Deep links preserve filters and absolute incident time.
- Copy a record, copy a support link, and export selected data.
- Display truncation, retention, and partial-result warnings.
- Helpful no-results state that explains active filters and time range.
- Bounded retention, rotation, rate limiting, and virtualization.

Mature references include
[Elastic's log explorer](https://www.elastic.co/guide/en/observability/current/explore-logs.html),
[Grafana's log integration](https://grafana.com/docs/grafana-cloud/visualizations/explore/logs-integration/),
and [Datadog facets](https://docs.datadoghq.com/logs/explorer/facets/). The goal is not to recreate
these products, but to adopt proven filtering, context, and drill-down behavior.

### Redaction and support bundles

Routine UI and exports must never expose:

- Credentials.
- Session tokens or cookies.
- Authorization headers.
- Private payload fields.
- Personal identifiers not required for support.
- Secrets embedded in URLs.

Support-bundle flow:

1. Select problem/operation or time range.
2. Preview included categories.
3. Apply redaction by default.
4. Show what leaves the machine, why, retention, and recipient.
5. Let the user inspect or exclude categories.
6. Require explicit consent before upload.
7. Produce a stable bundle ID and local copy option.

### Performance behavior

- Virtualize long histories.
- Defer pretty-printing until expansion.
- Parse/filter heavy data off the main thread where needed.
- Batch incoming updates.
- Cap in-memory rows and disclose the cap.
- Pause expensive hidden-pane work.
- Keep a stable key per event.
- Do not update every visible row for every incoming frame.

---

## Strategic decisions and brand gate

CitadelOps should be redesigned as a **calm, exception-first command center for safely delegating
complex game operations**. It should not look or behave like a generic analytics dashboard, a noisy
game HUD, a cyberpunk terminal, or a wall of protocol telemetry.

The product already has unusually good foundations for a trustworthy modern interface: normalized
revisioned game state, deterministic intents, dry runs, resource claims, effect classification,
operation status, cancellation, receipts, automation metrics, and both readable and raw telemetry.
The redesign opportunity is primarily to make those capabilities understandable and controllable.

The most important decisions are:

1. Make **state, consequence, and next action** visible before metrics or logs.
2. Add a true **empire-wide Overview** that prioritizes exceptions, freshness, and upcoming actions.
3. Turn Automations into a **control tower** that distinguishes enabled, waiting, running, blocked,
   retrying, failed, and paused.
4. Separate operational information into five levels: current state, attention item, human-readable
   activity, execution trace, and raw log.
5. Keep raw logs searchable and powerful, but secondary and prefiltered from the failure or
   operation that led to them.
6. Treat the public website, account portal, and desktop app as one brand ecosystem with different
   jobs—not one repeated layout.
7. Separate billing, subscription, entitlements, licensed devices, and authenticated sessions in
   both the model and the UI.
8. Build AI as a constrained copilot over the existing intent system: observe, explain, preview,
   dry-run, request approval, execute through the same path, and produce receipts.
9. Replace the current green-everywhere glass aesthetic with a neutral-first system where
   brand/action, success, warning, danger, information, stale, and disabled are distinct.
10. Establish semantic design tokens, accessible component contracts, route-level loading, scoped
    state subscriptions, and performance budgets before reskinning every screen.

### Critical brand finding

Preliminary web research found an operating South Florida IT company using the exact name
**CitadelOps** for enterprise IT consulting, security, cloud, infrastructure monitoring, and
automation at [citadelops.com](https://www.citadelops.com/). Those services appear materially
adjacent to the intended public positioning. This does not decide ownership or infringement, but it
makes professional name and trademark clearance a **P0 gate before major public-site, logo, or
marketing investment**.

The [USPTO explains](https://www.uspto.gov/trademarks/search/likelihood-confusion) that likelihood
of confusion considers similarity in appearance, sound, meaning, and commercial impression together
with related goods or services. A distinctive new symbol does not automatically cure a word-mark
problem. A comprehensive clearance process should cover federal, state, common-law, domain, social,
and international sources, guided by qualified counsel; a quick web or federal database search is
not clearance. See the USPTO's
[comprehensive clearance guidance](https://www.uspto.gov/trademarks/search/comprehensive-clearance-search-similar-trademarks)
and the [WIPO Global Brand Database](https://www.wipo.int/en/web/global-brand-database).

Until that gate is resolved, this report uses “CitadelOps” as the working product name and treats
all icon work as a reversible visual hypothesis.

### North-star promise

> **Calm command over complex game operations.**

The user should be able to answer these questions in seconds:

- Is the system connected, current, licensed, and safe?
- What needs my attention?
- What is automation doing now?
- Why is it doing or not doing that?
- What will happen next, when, and with which resources?
- How do I pause, change, inspect, or recover it?
- What evidence and receipt exist if something goes wrong?

### Recommended experience architecture

| Surface                    | Primary job                                                                     | Visual expression                 | Must not become                                      |
| -------------------------- | ------------------------------------------------------------------------------- | --------------------------------- | ---------------------------------------------------- |
| Public website             | Explain value, safety, compatibility, pricing, and download                     | Expressive, spacious, product-led | A vague fantasy landing page or an account dashboard |
| Account portal             | Manage entitlement, devices, billing, security, downloads, privacy, and support | Quiet, explicit, trust-oriented   | An upsell funnel that obstructs account tasks        |
| Desktop operations console | Monitor, decide, delegate, inspect, and recover                                 | Dense but calm, exception-first   | A marketing surface, game HUD, or raw log viewer     |
| AI copilot                 | Summarize, recommend, draft, explain, and assist controlled execution           | Contextual and subordinate        | An opaque chat agent with a separate command path    |

---

## How this research was conducted

This document combines:

- A repository and architecture audit of the active CitadelOps 2.0 worktree.
- A live visual inspection of the current Castle, Automation, and Logs experiences in dark mode.
- Current primary standards and official guidance from W3C, NIST, NASA, NRC, Microsoft/Xbox, Apple,
  USWDS, GOV.UK, OWASP, OAuth, FTC, USPTO, WIPO, OpenTelemetry, and established design systems.
- Product-pattern research from automation tools, workflow engines, observability systems, game
  companion tools, and incident-management products.
- App-specific synthesis rather than direct imitation of any one reference product.

The source set was weighted in this order:

1. Normative standards and government guidance.
2. Peer-reviewed research and human-factors guidance.
3. Official product and design-system documentation.
4. Mature product patterns used as analogues.
5. General design commentary only where primary evidence was not available.

No video analysis was necessary to establish the recommendations in this version. Written standards
and official product documentation provided more precise, auditable requirements. Videos may still
be useful later for studying interaction timing, onboarding, and motion after a prototype exists.

### Research boundaries

- This is UX and product guidance, not legal, security, privacy, billing, or accessibility
  certification.
- Game-platform rules, automation policies, licensing law, and subscription requirements can change
  by region and must be checked before launch.
- Color associations are contextual and cultural, not universal psychological laws.
- The concept palette and icon direction must be tested with users and cleared legally before
  becoming final identity assets.
- Performance numbers measured in the current worktree are a point-in-time development snapshot, not
  a production benchmark.

---

## The complete UI-design problem space

A redesign of this product is not mainly a question of color or component style. It is a coordinated
system of product, human, visual, technical, commercial, and operational decisions.

### 1. Product strategy

- Audience and expertise levels.
- Core jobs, outcomes, and risks.
- Product promise and differentiation.
- Local versus cloud responsibilities.
- Free, trial, licensed, and unavailable states.
- Safety boundaries and product ethics.
- Success metrics and business constraints.

### 2. User research

- Novice versus expert mental models.
- Current workflows and workarounds.
- Frequency, importance, difficulty, and consequence of tasks.
- Context of use: long-running desktop session, second monitor, narrow window, poor connectivity,
  high game activity.
- Vocabulary players already use.
- Confidence, workload, trust, and recovery behavior.
- Accessibility needs and input preferences.

### 3. Information architecture

- Public, account, desktop, and support boundaries.
- Navigation groups and depth.
- Scope: empire, account, castle, automation, execution, step.
- Durable URLs and deep links.
- Search, saved views, filters, and recent locations.
- Advanced/developer boundaries.

### 4. Interaction architecture

- Primary tasks and secondary evidence.
- Selection versus side-effecting commands.
- Progressive disclosure.
- Keyboard, pointer, controller-like digital input, and assistive technology.
- Direct manipulation versus forms or commands.
- Undo, confirmation, cancellation, pause, stop, and recovery.
- Optimistic, queued, acknowledged, and confirmed states.

### 5. Automation human factors

- Mode awareness.
- Enabled versus actively executing.
- Current step, rationale, next transition, and time.
- Human/automation responsibility.
- Resource contention and claims.
- Safe override and emergency stop.
- Retry, backoff, idempotency, and redrive.
- Traceability to an input snapshot and definition version.

### 6. Content design

- Plain-language naming.
- Status vocabulary.
- Error and recovery messages.
- Numbers, units, dates, time zones, and relative time.
- Onboarding and contextual help.
- Empty, loading, stale, partial, offline, and unsupported messages.
- Localization and bidirectional text.

### 7. Visual hierarchy

- What is true now.
- What needs attention.
- What happens next.
- Primary action.
- Supporting evidence and metrics.
- Raw evidence.
- Page rhythm, grouping, whitespace, density, and elevation.

### 8. Color theory and system

- Perception and contrast.
- Semantic roles.
- Brand association.
- Data-visualization scales.
- Light, dark, high-contrast, and forced-color themes.
- Color-vision diversity.
- Translucency and real-background testing.
- Gamut and perceptual color spaces.

### 9. Typography

- Type family and brand voice.
- Productive versus expressive scale.
- Numerals, coordinates, timers, and tables.
- Line length and reading rhythm.
- Weight, size, spacing, and case hierarchy.
- User text-spacing overrides and localization expansion.

### 10. Iconography and brand identity

- Positioning and personality.
- Naming and trademark risk.
- Symbol distinctiveness.
- Small-size icon construction.
- Wordmark, app icon, favicon, tray icon, and monochrome systems.
- Illustration, game art, and marketing expression.
- Tone of voice and motion personality.

### 11. Data visualization

- Decision served by each metric.
- Correct chart form and scale.
- Targets, thresholds, comparisons, and timestamps.
- Accessible alternative summaries.
- Interaction without hover dependency.
- Empty, incomplete, and stale data.

### 12. Logs and observability

- Human activity history.
- Actionable alerts.
- Execution traces.
- Structured diagnostics.
- Retention, volume, redaction, export, and correlation.
- Live-tail behavior and scroll preservation.
- Performance under sustained updates.

### 13. Accessibility and inclusive design

- WCAG 2.2 AA baseline.
- Game-adjacent accessibility guidance.
- Keyboard and focus behavior.
- Screen-reader structure and live updates.
- Text resize and reflow.
- Motion, transparency, contrast, and forced-color preferences.
- Touch targets, cognitive load, error prevention, and recovery.

### 14. Performance and resilience

- Startup and route-loading cost.
- Interaction latency and frame budget.
- Render and update frequency.
- Image, catalog, and state payload cost.
- Virtualization and background work.
- Offline, partial failure, and degraded mode.
- Memory growth, retention, and long-running sessions.

### 15. Design-system engineering

- Primitive, semantic, and component tokens.
- Component contracts and state matrices.
- Density, themes, responsive behavior, and localization.
- Ownership, versioning, deprecation, and quality gates.
- Visual regression, accessibility, and performance coverage.

### 16. Account, licensing, and commerce

- Authentication and recovery.
- Subscription versus entitlement.
- Billing, taxes, invoices, renewals, cancellation, and refunds.
- Device allocation versus sessions.
- Grace, past-due, revoked, expired, and offline-license states.
- Honest pricing and non-coercive cancellation.

### 17. Trust, privacy, security, and support

- Local/cloud data map.
- Game credential handling.
- Telemetry categories and consent.
- Signed updates and release provenance.
- Status page and incident communication.
- Redacted support bundles.
- Export, deletion, retention, and vulnerability reporting.

### 18. Measurement and governance

- Task completion, time, and errors.
- Correct state and scope comprehension.
- Workload, confidence, and trust calibration.
- Duplicate or unsafe command rate.
- Alert quality and recovery success.
- Accessibility and performance budgets.
- Decision logs and design-system adoption.

---

## Foundational interaction and visual-perception theory

These concepts are not decoration advice. They explain why certain structures reduce effort and
error in an operations console.

### Gestalt grouping

People perceive relationships through proximity, similarity, common region, continuity,
connectedness, and common fate before reading every label.

CitadelOps implications:

- Group status with the castle or automation it describes; do not place a detached badge where its
  owner is ambiguous.
- Use common region for one operational object, not a card around every fragment.
- Use similarity for truly equivalent controls; do not style Pause and Delete alike.
- Use explicit connectedness in execution traces and timelines.
- Separate unrelated healthy telemetry from the attention queue even if both contain “status.”
- Preserve grouping when the layout collapses responsively.

NN/g's discussion of the
[connectedness principle](https://www.nngroup.com/videos/connectedness-gestalt/) illustrates how a
line or connection can override weaker differences. In CitadelOps, that makes step-to-step trace
connections powerful and also means careless connectors can imply a false causal relationship.

### Affordances and signifiers

An affordance is what an object permits; a signifier indicates where and how to act. A soft panel
that looks like every other panel but is secretly clickable has a weak signifier.

Rules:

- Buttons, links, toggles, rows, and disclosures have consistent visible interaction cues.
- A whole row is clickable only when focus, hover, cursor, and keyboard behavior communicate it.
- Disabled controls still explain their unavailable action and reason.
- Drag handles appear only where drag is supported, with a non-drag alternative.
- Decorative badges and status labels do not resemble controls.

### Recognition rather than recall

Users should not remember a status explanation from a tooltip, a castle ID from another page, or a
retry rule from setup. Keep required context visible or directly retrievable. This is one of
[NN/g's core heuristics](https://www.nngroup.com/articles/recognition-and-recall/).

Applications:

- Show the active scope beside an action.
- Show the current preset, reserve, commander, and schedule at review time.
- Preserve filter chips with search results.
- Put the failed condition beside the failed step.
- Provide recent entities and saved views rather than requiring exact names.

### Cognitive load and progressive disclosure

Intrinsic complexity cannot be removed from a game-operations product, but avoid adding extraneous
load through inconsistent vocabulary, competing colors, mode ambiguity, hidden state, and repeated
navigation. Break a complex plan into meaningful chunks while keeping its overall consequence
visible.

Progressive disclosure is not “hide everything.” Common controls remain visible. Expert detail is
deferred only when the user can discover it and keep context. NN/g's critique of indiscriminate
[Zen mode](https://www.nngroup.com/articles/zen-mode/) explains how hiding useful chrome can
increase memory, interaction, and attention-switch costs.

### Target acquisition and Fitts's law

The time and effort to reach a target depend on its size and distance. CitadelOps implications:

- Make frequent, urgent, and destructive targets comfortably large and separated.
- Keep an attention item's recovery action near the item.
- Do not place a tiny master Pause target among unrelated header telemetry.
- Use screen edges intentionally for stable global navigation, not tiny floating handles alone.
- Preserve generous targets even in Compact density.

WCAG target-size criteria provide accessibility thresholds; Fitts's law explains the broader
interaction-cost reason.

### Choice complexity

More choices do not always produce a proportionally slower decision; labels, grouping, familiarity,
defaults, and consequence matter. Do not invoke “Hick's law” as a reason to hide every option.

Applications:

- Group automations by player goal and state.
- Offer safe defaults and templates.
- Put the recommended recovery first and explain why.
- Compare materially different plans in a table rather than a long menu.
- Remove duplicate or impossible actions for the current state.

### Preattentive attributes and signal economy

Position, size, enclosure, orientation, and limited color can be noticed before deliberate reading.
Reserve these for meaningful differences. If everything glows, pulses, or uses a colored pill,
nothing remains preattentive.

Use:

- Stable position for attention and master control.
- Stronger size/weight for current consequence.
- Enclosure for an actionable exception.
- Color only for a small number of semantic states.
- Motion only for causality or a genuinely time-sensitive transition.

### Change blindness and live updates

Users can miss changes, especially when many elements update simultaneously or a change occurs
outside focus. Conversely, animating every update destroys attention.

Rules:

- Preserve layout position during live updates.
- Mark only material changes briefly and locally.
- Summarize high-frequency updates into current state.
- Keep a durable activity record.
- Announce meaningful status changes accessibly without narrating the stream.
- Use “new events” affordances instead of moving a reader's scroll position.

### Feedback, feedforward, and closure

- **Feedforward:** Before action, show scope, cost, constraints, and likely outcome.
- **Immediate feedback:** Acknowledge the input locally.
- **Progress feedback:** Show queued, sent, acknowledged, and confirmed states as supported.
- **Closure:** Provide a receipt that says what changed.
- **Recovery:** Preserve enough context to retry safely or choose another path.

The existing intent, operation, and receipt architecture can support all five.

---

## Product and repository audit

### Product model to preserve

CitadelOps is more capable than the current surface suggests:

- A normalized, revisioned state spans castles, commanders, movements, events, reports, automations,
  inventory, maps, and observations.
- Mutations are designed to pass through a deterministic intent engine.
- The intent layer can classify effects, dry-run, claim resources, require expected revisions,
  create plans, expose status, cancel, and return receipts.
- Fourteen server-side automation policies are registered in the current snapshot.
- Telemetry supports readable activity and exact wire evidence.
- The local Go process serves the React desktop interface and manages a dedicated Chromium game
  session.

This is an excellent foundation for an AI-assisted interface because AI can propose **deterministic,
reviewable intents** instead of inventing a parallel execution mechanism.

### Primary user jobs

1. Connect the game and know whether data is live, delayed, stale, incomplete, or offline.
2. See empire-wide health, upcoming pressure, queues, resources, movements, and exceptions.
3. Enable automation with a correct understanding of scope, dependencies, budgets, and stop
   conditions.
4. Know what each automation is doing, why it is waiting, and what it will do next.
5. Manage shared scarce resources such as commanders, slots, currency, troops, tools, and time
   skips.
6. Perform high-value manual combat, defense, spying, equipment, and Rift work.
7. Review event, battle, player, and alliance intelligence.
8. Recover from blocked or failed work without duplicating completed purchases or actions.
9. Produce a useful support record without exposing secrets or reading protocol frames.
10. Pause automation immediately without confusing that action with disconnecting the game.

### Current interface strengths

- Game-specific vocabulary and art provide recognition.
- Attack and defense presets encode reusable expertise.
- Equipment and analytics are rich task workspaces.
- Connection handling already recognizes stale and retry states.
- The logger already has readable/raw modes, search, filters, expandable data, copying, and
  virtualization.
- Automation is centrally coordinated and a global safety lock exists.
- Some component primitives include semantic status tokens, focus-visible styling, alert roles, and
  reduced-motion support.

These should be refined, not discarded.

### Current information architecture

The current sidebar is a flat set of roughly fourteen destinations. Navigation is held in local
React state rather than durable routes, so locations cannot reliably support browser history, direct
links, bookmarks, or links from an alert to a specific operation.

The default Castle screen is a focused-castle workspace, not an empire overview. Events currently
behave more like a compact dashboard module than a complete top-level workspace. Presets are
separated even though they share a library mental model. Settings combines system/browser behavior,
attack arbitration, and equipment preferences. Support exposes a raw JSON Intent Console above human
support.

### Current Automation gap

The server already exposes concepts such as:

- Status.
- Explanatory detail.
- Next check time.
- Last run time.
- Last operation ID.
- Last error.
- Policy metrics.
- Update time.

The coordinator distinguishes disabled, waiting, scheduled, blocked, running, idle, and error
states. The current Automation screen reduces most of this to an enabled switch, a description, a
settings button, and a count badge. Important rationale is frequently relegated to a native hover
title.

The result is a trust gap: a user can turn something on but cannot immediately see whether it is
working, waiting, blocked, stale, or safe.

### Current header gap

The global header combines:

- Dashboard/API connection.
- Browser lifecycle.
- Game WebSocket and login.
- Castle focus.
- Two individual automations.
- The global automation lock.
- Session start and stop.
- Memory readings.

Healthy status therefore competes visually with actionable status, while different consequences use
opaque labels such as “Start Bot,” “Stop Bot,” and “Lock Bot.”

Prefer explicit concepts:

- **Connect game** / **Disconnect game**.
- **Pause all automations** / **Resume automations**.
- **View castle** versus **Focus this castle in game**, if those remain separate effects.

Normal healthy state should collapse into one quiet summary. A degraded summary can expand into the
connection chain that explains which layer failed.

### Current visual audit

The live interface uses slate surfaces, emerald as both brand and operational emphasis, ambient
green gradients, translucent glass, repeated blur, large shadows, and a globally soft radius.

Observed issues:

- Emerald action/brand and green success are too similar.
- Red “off” indicators imply danger for a neutral inactive state.
- Many simultaneous pills and status fragments compete for attention.
- Critical runtime explanation is missing from the Automation rows.
- Large areas of empty space coexist with tiny 10–11 pixel text.
- Glass and glow are repeated across nearly every layer, reducing hierarchy.
- The global radius rule appears to override shape semantics, including intended circles and pills.
- Global letter-spacing suppression removes a useful typographic hierarchy tool and can conflict
  with user spacing overrides.
- The dark theme is visually dominant without a System theme choice.
- The current hexagon, growth bars, and trend line read as generic analytics, fintech, or
  infrastructure monitoring more than a game-operations product.

### Current accessibility risks

Positive foundations exist, but foundational components need work before screen-by-screen polish:

- A modal primitive lacks a complete focus trap, initial focus, focus return, Escape handling, and
  explicit accessible-title association.
- A custom select lacks full combobox/listbox semantics and keyboard behavior.
- Sidebar destinations rely on `div` elements with button roles rather than durable links or native
  buttons, and current location is not exposed with `aria-current`.
- Toggle groups do not consistently expose pressed state.
- Some icon buttons are around 32 pixels rather than a comfortable target.
- Important explanations depend on hover titles.
- Small muted text is common.
- Wide tables assume a desktop viewport and horizontal scrolling.

### Current performance risks

Point-in-time, on-disk measurements in this active development snapshot found the following. These
are raw filesystem sizes, not compressed network transfers or runtime-memory measurements. The
`Client/dist` total includes the copied `public/game-data` directory, so it must not be interpreted
as JavaScript/CSS bundle weight alone.

| Artifact                  |     Approximate size |
| ------------------------- | -------------------: |
| `Client/public`           |               228 MB |
| `Client/public/game-data` | 3,049 files / 228 MB |
| Decoration assets         |               206 MB |
| Built client              |               268 MB |
| Desktop binary            |               115 MB |
| Main JavaScript bundle    |           831 KB raw |
| Main CSS bundle           |           245 KB raw |

Structural concerns include:

- Major workspaces are eagerly imported, including very large analytics views.
- Only one obvious feature is emitted as a separate client chunk.
- The global metadata provider loads many catalog/projection sources at startup.
- State-change events can trigger broad state refetches.
- One broad state context can cause unrelated consumers to update.
- Movement and update polling can run while their views are inactive.
- Operation receipt maps need an explicit retention/pruning policy.
- Sustained blur, shadow, and large translucent surfaces may add GPU and repaint cost.

### Repository evidence map

The following single reference block identifies the main code evidence used in the audit. Line
numbers describe the inspected worktree and may drift as active development continues.

```code-reference
R1  /Users/nebulabot/Desktop/CitadelOpsDesktop/Architecture.md:7-29
R2  /Users/nebulabot/Desktop/CitadelOpsDesktop/Architecture.md:68-94
R3  /Users/nebulabot/Desktop/CitadelOpsDesktop/Architecture.md:142-169
R4  /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Automation/Coordinator.go:189-360
R5  /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Automation/Coordinator.go:474-513
R6  /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Intent/Types.go:14-156
R7  /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/api/Contracts.ts:922-933
R8  /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/api/Contracts.ts:1126-1161
R9  /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/App.tsx:4-213
R10 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/config/Navigation.tsx:27-42
R11 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/components/Header.tsx:246-394
R12 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/views/AutomationView.tsx:335-399
R13 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/components/LoggerDock.tsx:301-417
R14 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/components/LoggerDock.tsx:460-704
R15 /Users/nebulabot/Desktop/CitadelOpsDesktop/Server/Telemetry/Store.go:17-57
R16 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/components/IntentConsole.tsx:9-123
R17 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/index.css:5-187
R18 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/index.css:243-468
R19 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/components/ui/Modal.tsx:16-98
R20 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/components/ui/Select.tsx:29-216
R21 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/context/MetadataContext.tsx:51-193
R22 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/api/ApiContext.tsx:116-184
R23 /Users/nebulabot/Desktop/CitadelOpsDesktop/Client/src/Movement/context/MovementContext.tsx:15-47
```

---

## Comparative product-pattern analysis

References are used as pattern evidence, not visual templates.

| Reference                                                                                                   | Adopt                                                                        | Reject / avoid                                                        | CitadelOps translation                                                                         |
| ----------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | --------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| [Home Assistant automations](https://www.home-assistant.io/docs/automation/editor/)                         | Trigger–condition–action simplicity; traces; templates                       | Configuration sprawl and domain-specific terminology                  | Guided When/If/Then editor with current GameState dry run and player vocabulary                |
| [AWS Step Functions](https://docs.aws.amazon.com/step-functions/latest/dg/workflow-studio.html)             | Versioned definitions, validation, state testing, exact failed step, redrive | Cloud-engineering density and graph-first complexity for simple tasks | Advanced execution graph behind a plain recipe editor; safe resume without duplicate purchases |
| [n8n executions](https://docs.n8n.io/workflows/executions/all-executions/)                                  | Searchable run states, reuse prior input, execution-centric diagnosis        | Node-canvas dependence for every workflow                             | Filterable operation history and “start revised execution from this input”                     |
| [Node-RED](https://nodered.org/docs/user-guide/editor/)                                                     | Secondary debug panel, node/runtime status, pause/clear/filter               | Raw message flow as the primary player experience                     | Remembered diagnostics dock and visible feature status, with protocol detail deferred          |
| [Grafana Explore](https://grafana.com/docs/grafana/latest/visualizations/explore/get-started-with-explore/) | Dashboard-to-investigation context, surrounding logs, live-tail control      | Metric/dashboard sprawl and query-language exposure to normal users   | Attention item → operation → prefiltered diagnostics with human labels                         |
| [Elastic log exploration](https://www.elastic.co/guide/en/observability/current/explore-logs.html)          | Structured fields, include/exclude filters, row expansion, context           | Making log search the home page                                       | Expert Diagnostics route beneath Activity and execution traces                                 |
| [Destiny Item Manager](https://destinyitemmanager.com/en/)                                                  | Character/account scope, saved searches, loadouts, recommendation context    | Assuming a companion tool can use game-HUD density everywhere         | Visible castle scope, reusable presets, task-specific dense workspaces                         |
| [RuneLite profiles](https://runelite.dev/blog/show/2023-02-18-1.9.11-Release/)                              | Profiles, duplicate/import/export, activity-specific settings                | Plugin-market trust and compatibility complexity in the core product  | Versioned account/castle profiles and explicit advanced-extension boundary if added later      |
| Mature account portals                                                                                      | Explicit plan, invoices, devices, sessions, recovery                         | Upsells obstructing security, billing, and cancellation               | Quiet account portal separated from desktop operations and marketing                           |

### What this comparison implies

- The automation editor should be simple by default and trace-rich after execution.
- The Overview should link into investigation but never become an observability product.
- Presets and profiles are first-class game-companion concepts worth preserving.
- Technical capability must be translated into game nouns, scope, consequence, and recovery.
- Public commerce and account management share identity with the desktop but not information
  density.

---

## Experience principles

These principles should become design-review criteria.

### 1. State before decoration

The first visual layer communicates what is true, whether it is current, and whether action is
needed. Decorative atmosphere never competes with operational status.

### 2. Explain automation, do not mystify it

Every automation exposes current state, reason, scope, input freshness, current or last step, next
expected action/time, and a safe control path.

NASA's current
[Crew Interfaces guidance](https://www.nasa.gov/reference/10-0-crew-interfaces-vol-2/) is a useful
high-reliability analogue: automation interfaces should expose state, projected future state,
health, configuration, operation/performance information, responsibility boundaries, mode changes,
and safe override/shutdown. CitadelOps is not safety-critical, but the human-factors
problem—preventing automation surprise—is directly relevant.

### 3. Exceptions interrupt; routine activity accumulates

If an event has no meaningful user action, it is not an alarm. Routine success goes to activity
history. Persistent attention is reserved for conditions with consequence and a recovery action.

### 4. Overview first, evidence on demand

The interface proceeds from overview to scoped detail to execution trace to raw diagnostics.
Progressive disclosure should reduce cognitive load without hiding common controls.
[NN/g's guidance](https://www.nngroup.com/articles/progressive-disclosure/) supports prioritizing
common and important options while deferring rare complexity.

### 5. Scope is never implicit

Every broad action names the affected empire, account, castle, automation, execution, or resource
set before execution.

### 6. Enabled is not running

The design never uses one on/off switch to represent configuration, scheduling, execution, health,
and completion. These are separate dimensions.

### 7. Last known is not live

Cached data remains useful during failure, but it is labeled with freshness and cannot silently
authorize unsafe work.

### 8. Pause is not disconnect

Session connectivity, scheduling pause, stop-after-safe-step, immediate cancellation, and
disablement have distinct labels and consequences.

### 9. Human language first, protocol evidence last

Players should not need opcodes or JSON to understand an outcome. Technical identifiers remain
available for expert support.

### 10. AI shares the same rules

AI has no secret write channel. It observes the same state, drafts or submits the same typed
intents, respects the same budgets and locks, and returns the same operation evidence.

### 11. Accessible by construction

Color, keyboard behavior, focus, text size, reflow, motion, names, and live updates are component
contracts, not a cleanup phase.

### 12. Modern means coherent

Modernity comes from clear hierarchy, precise type, responsive feedback, honest state, and
disciplined motion—not from glass everywhere, oversized bento cards, neon, or pill-shaped
everything.

---

## Users, contexts, and risk model

### Core user modes

| Mode                 | Need                                           | Design response                                              |
| -------------------- | ---------------------------------------------- | ------------------------------------------------------------ |
| First-time user      | Safety, vocabulary, a successful first outcome | Guided setup, starter automation, dry run, visible pause     |
| Routine operator     | Fast exception scan and minimal interruption   | Quiet overview, attention queue, saved scope, keyboard paths |
| Optimizer            | Dense comparison and reusable strategies       | Compact density, presets, saved views, bulk tools, analytics |
| Investigator         | Cause, evidence, context, recovery             | Operation trace, correlated activity, filtered diagnostics   |
| Account owner        | License, devices, billing, security            | Separate portal with explicit state and consequences         |
| Support collaborator | Reproducible, redacted evidence                | Consentful support bundle and correlation links              |

### Context assumptions to validate

- CitadelOps may run for hours or days.
- The user may glance at it on a second monitor.
- The game and dashboard may be in different connection states.
- The window may be narrow even on a desktop.
- Experts need high density, while new users need explanation.
- Some actions spend scarce or purchased resources.
- Network, game protocol, license service, and local process failures can occur independently.
- A single visible failure may be caused by one upstream condition affecting many automations.

### Action-risk classes

| Class                           | Examples                                          | Default interaction                                                  |
| ------------------------------- | ------------------------------------------------- | -------------------------------------------------------------------- |
| Read                            | Inspect state, search logs, preview a plan        | Immediate                                                            |
| Reversible local write          | Save a view, change density, edit a draft         | Immediate with undo where useful                                     |
| Scheduling/configuration        | Enable a policy, change scope, alter a reserve    | Review affected scope; confirmation only if consequence is material  |
| Game write                      | Equip, recruit, upgrade, move resources           | Preview outcome/cost; submit; show queued/confirmed receipt          |
| Launch or conflict-prone action | Attack, use shared commander, consume attack slot | Explicit scope, claims, preconditions, confirmation                  |
| Purchase/high-value action      | Spend premium currency, buy/upgrade               | Explicit amount/budget, effect, non-idempotency, strong confirmation |
| External/account action         | Revoke device, cancel renewal, delete data        | Exact consequence and durable confirmation                           |

---

## Target product ecosystem and information architecture

The redesign should use one identity and design system across three coordinated products.

### Public website

Recommended public site map:

- Home.
- Product / How it works.
- Automations and use cases.
- Safety and transparency.
- Pricing.
- Download.
- Documentation and guides.
- Changelog.
- Security and privacy.
- Support.
- Status.
- Sign in.

Keep the primary header to roughly five high-value links—**Product, Automations, Pricing, Download,
and Docs**—plus **Sign in** and a visually clear download action. Place Safety/Security, Changelog,
Support, Status, Privacy, and legal destinations in a utility area or footer, while linking them
contextually from relevant pages.

The public site should explain:

- Who the product is for.
- What it automates and what remains under user control.
- Supported platforms and environments.
- How scope, budgets, dry runs, pause, and receipts work.
- What data remains local and what, if anything, reaches cloud services.
- Pricing, device limits, trial behavior, and renewal terms before account creation.
- Download provenance, system requirements, release date, version, and publisher.
- Honest limitations and game-provider dependencies.

### Account portal

Recommended navigation:

- Overview.
- Devices and installations.
- Plan, billing, and invoices.
- Security and sessions.
- Downloads and releases.
- Support tickets.
- Data and privacy.

The account overview should prioritize entitlement and exceptions:

- Current plan and active entitlements.
- Trial/renewal/end date and exact next charge where applicable.
- Device slots used and available.
- Billing problems and grace deadline.
- Security issues or unfamiliar sessions.
- Latest stable desktop release.
- Incidents relevant to the user.

Marketing upsells should not compete with payment repair, recovery, device revocation, or privacy
tasks.

### Desktop application

Recommended navigation:

- **Overview**
  - Today / Empire health.
  - Castles.
  - Events.
- **Automations**
  - Control center.
  - Schedules.
  - Commander and attack-slot allocation.
  - Templates and definitions.
- **Combat**
  - Attack setup.
  - Defense.
  - Movements.
  - Equipment.
- **Intelligence**
  - Battle reports.
  - My performance.
  - Alliance targets.
  - Rift.
- **Library**
  - Attack presets.
  - Defense presets.
- **System**
  - Connection and browser.
  - Activity.
  - Diagnostics.
  - Preferences.
  - Updates.
  - Support.
  - Developer tools.

This grouping should be validated with card sorting and tree testing rather than adopted by taste
alone. [NN/g's IA study guide](https://www.nngroup.com/articles/ia-study-guide/) describes
appropriate methods.

### Routing and deep links

Move from view state to durable routes. Examples:

```text
/overview
/castles/:castleId
/automations
/automations/:automationId
/automations/:automationId/executions/:operationId
/activity?actor=auto-tci&castle=:castleId&from=:time
/diagnostics/logs?operation=:operationId
/library/attack-presets/:presetId
/system/connection
```

Requirements:

- Browser history works.
- Refresh preserves location.
- Notifications deep-link to the relevant scope.
- Filters are shareable when safe.
- Incident links use absolute time, not only “last 15 minutes.”
- Sensitive query values are avoided.
- The selected castle and live in-game focus are modeled separately if they have different side
  effects.

---

## Public website specification

### Public-site job

The website should move a visitor from **understanding** to **trust** to **fit** to **download**,
not merely display a dramatic hero and feature grid.

The public site must answer:

1. What is CitadelOps?
2. Which game workflows does it help with?
3. What remains under my control?
4. How does it avoid unsafe or duplicate actions?
5. What data stays local or leaves my machine?
6. What platforms and game environments are supported?
7. What does it cost, what are the limits, and when would I be charged?
8. How do I download, install, sign in, and get a first result?
9. What happens during an outage or game-provider change?
10. Where can I find documentation, security details, support, and service status?

### Recommended public pages

#### Home

- One-sentence promise.
- Authentic product proof, not an abstract fake dashboard.
- Three outcome pillars: supervise, automate, understand.
- Safety/control proof: preview, budgets, pause, receipts.
- Supported environment and current release.
- Clear **Download app** and **See how it works** actions.
- Pricing summary without hiding unavoidable cost.
- Trust links: Security, Status, Documentation, Changelog.

#### Product / How it works

Use a concrete four-part story:

1. Citadel observes and normalizes live state.
2. Policies evaluate goals, constraints, and shared resources.
3. Plans are previewed or executed through deterministic intents.
4. Every outcome has activity, trace, and receipt evidence.

Include the role of the local desktop process, managed browser, game provider, and optional
account/cloud services. Avoid suggesting that an AI model directly controls the game.

#### Automations and use cases

Group by player goal rather than internal module name alone:

- Keep production and construction moving.
- Protect resource and food reserves.
- Coordinate recruiting and recovery.
- Manage events and targets.
- Prepare and launch combat workflows.
- Optimize equipment and reusable presets.

Each example should show **trigger → guardrails → action → evidence**, including one case where the
correct outcome is to wait.

#### Safety and transparency

- State freshness and stale-data behavior.
- Resource budgets and prohibited action classes.
- Dry runs and scoped previews.
- Pause, stop, and cancellation semantics.
- Shared-resource claims.
- Execution history and receipts.
- AI permission levels and human approval.
- What the product cannot guarantee.

This page is a differentiator, not a legal footnote.

#### Pricing

- Total recurring price and currency.
- Billing interval and any taxes/fees treatment.
- Included entitlements and limits.
- Device/install slots.
- Trial length and exact first-charge behavior.
- Cancellation behavior and access end date policy.
- Comparison based on outcomes and limits, not arbitrary feature fog.
- Link to billing support and terms.

#### Download

- Auto-detect likely OS, but always show other platforms/builds.
- Version, release date, stable/beta channel, and system requirements.
- Signed publisher and install-verification guidance.
- Release notes and checksum where useful.
- Troubleshooting and known compatibility issues.
- A continuing guide: **Install → Sign in → First safe automation**.

Consistent code signing and publisher reputation matter to Windows download trust; see
[Microsoft SmartScreen guidance](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/smartscreen-reputation).
Update delivery should include cryptographic integrity verification and a threat model for
repository or signing-key compromise. [The Update Framework](https://theupdateframework.io/) is a
mature reference architecture; the exact implementation must fit the desktop distribution model.

#### Documentation

- Getting started.
- Automation concepts.
- Scope, schedules, budgets, and stop conditions.
- Connection model.
- Activity, traces, and diagnostics.
- Account and licensing.
- Accessibility.
- Troubleshooting and known issues.
- API/advanced material behind an explicit technical section.

#### Security and privacy

- Local versus cloud data map.
- Game credential/token handling.
- Encryption in transit/at rest where applicable.
- Signed update process.
- Account security options.
- Required operational telemetry versus optional analytics/crash reports.
- Retention and deletion.
- Incident/status link.
- Vulnerability disclosure/contact.
- Last updated date.

Avoid vague claims such as “military-grade security.” State verifiable controls and boundaries.

#### Status

Public and unauthenticated. Suggested components:

- Citadel account/cloud API.
- Authentication.
- Billing.
- Downloads and updates.
- Desktop connectivity service, if applicable.
- Upstream game connectivity.

Clearly separate Citadel-controlled failures from the upstream game provider.
[Atlassian incident communication guidance](https://www.atlassian.com/incident-management/incident-communication)
recommends a dedicated source of truth and layered communication by urgency.

#### Support

- Getting started.
- Automation.
- Account/game connectivity.
- Install/update.
- Billing/licensing.
- Troubleshooting.
- Known issues.
- Contact support.

Support should be available without opening raw developer tools.

### Public-site visual expression

- More expressive type scale than the app.
- Controlled game-world atmosphere at hero/section boundaries.
- Real product states and readable proof.
- Richer gradient or wide-gamut accent only in noncritical illustration.
- Generous whitespace and 16-pixel body baseline.
- A small number of signature shapes derived from the mark.
- Motion that demonstrates cause/effect, not ambient “tech energy.”

The marketing site, portal, and desktop should share identity tokens but not identical density or
layout. Carbon's
[productive and expressive type strategies](https://carbondesignsystem.com/elements/typography/style-strategies/)
provide a useful model for varying expression within one system.

### Discoverability and metadata

- Unique descriptive title and summary for every public page.
- Canonical URLs and a deliberate redirect policy.
- Sitemap and crawl rules that exclude private account/app surfaces.
- Structured data only when it truthfully matches visible content.
- Accessible headings and real text, not image-only marketing copy.
- Open Graph/social preview that reuses the actual brand, promise, and product proof.
- A favicon and app metadata built from the cleared icon system; see
  [Google's favicon guidance](https://developers.google.com/search/docs/appearance/favicon-in-search).
- Product, pricing, download version, security, and changelog content with visible update dates.
- No indexable private operation, support-bundle, or account URLs.

### Analytics and consent

- Define the minimum decisions analytics must support before choosing tools.
- Prefer aggregate, privacy-respecting measurement.
- Separate required security/operational telemetry from optional marketing analytics.
- Do not place advertising trackers inside the desktop operations console.
- Honor regional consent requirements and user choices.
- Avoid recording credentials, identifiers, form secrets, account screens, or private operations in
  replay tools.
- Publish retention and processor information.
- Make consent refusal a real option, not a degraded trick flow.

### Mobile and slow-network behavior

The public site and account portal must be fully useful on mobile and constrained networks even if
the desktop application itself targets desktop operating systems.

- Lead with text and primary action before large media.
- Use responsive, dimensioned images and progressive loading.
- Keep pricing, authentication, recovery, device revocation, invoices, support, and status usable at
  320 CSS pixels.
- Do not make a video the only product explanation.
- Preserve checkout/sign-in state across a connection interruption.
- Show download alternatives rather than assuming the visitor is on the installation device.
- Make incident and billing messages readable without decorative assets.

### Checkout and desktop handoff

```text
Pricing → plan/term review → account/auth → payment → entitlement confirmation
       → download/open app → browser authorization → named device → first safe automation
```

Requirements:

- Keep plan, term, total due, renewal, device limit, and cancellation behavior visible at review.
- Do not add a preselected paid add-on.
- Return to the intended plan after authentication/recovery.
- After payment, confirm entitlement—not only a receipt number.
- Offer Download and Open CitadelOps with manual fallback.
- Let the desktop refresh entitlement immediately.
- Send an invoice/receipt and an entitlement confirmation as distinct concepts where applicable.

### Lifecycle communication

Email or account notifications should cover meaningful durable events:

- Account verification and security change.
- New device/session and revocation.
- Trial start and advance notice of conversion.
- Payment failure, grace deadline, and recovery.
- Renewal, cancellation, refund, and access-end confirmation.
- Material terms or privacy change.
- Security-critical update or incident relevant to the user.
- Support ticket updates.

Each message states account, event, date/time, consequence, and a safe direct path. Do not use
urgent security styling for routine marketing, and do not hide operational notices inside
promotional campaigns.

### Public legal and footer architecture

Utility/footer navigation should provide:

- Privacy.
- Terms and subscription terms.
- Acceptable-use/game-policy disclosures as applicable.
- Cookie/analytics choices.
- Refund/cancellation policy.
- Security and vulnerability reporting.
- Status.
- Changelog.
- Support/contact.
- Company/legal identity and region-specific disclosures.

Legal review must determine final requirements; the UX obligation is to make material terms easy to
find and understand before commitment.

---

## Account, authentication, licensing, and billing

### Separate domain models

Do not use “license” as a catch-all. Model and present:

| Model                        | Question answered                                   |
| ---------------------------- | --------------------------------------------------- |
| Identity                     | Who is this user?                                   |
| Authenticated session        | Where are they currently signed in?                 |
| Subscription                 | What commercial agreement and renewal state exists? |
| Billing                      | Was payment setup/collection successful?            |
| Entitlement                  | Which features and limits are currently permitted?  |
| Licensed installation/device | Which installations consume device slots?           |
| Desktop/game session         | Which local app and game connection are active?     |

An account may have an active subscription, a failed payment in grace, two entitled devices, five
authenticated browser sessions, and one connected game session. One status word cannot represent all
of this.

### Authentication

Current [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html), finalized in 2025,
supports phishing-resistant authenticators such as passkeys/WebAuthn and rejects arbitrary
password-composition rules. CitadelOps should offer:

- Passkeys/WebAuthn as a preferred method.
- Password manager compatibility and paste.
- MFA and recovery codes.
- Recent security events.
- Active sessions with device/browser, last active, and current marker.
- Revoke one session or all other sessions.
- Accessible authentication without cognitive puzzles as the sole path; see
  [WCAG Accessible Authentication](https://www.w3.org/WAI/WCAG22/Understanding/accessible-authentication-minimum).

### Desktop sign-in handoff

Native/desktop authorization should use the system browser and Authorization Code with PKCE in line
with [OAuth for Native Apps](https://datatracker.ietf.org/doc/html/rfc8252). Use
[OAuth Device Authorization](https://datatracker.ietf.org/doc/html/rfc8628) only when a secure
callback is not practical.

The browser page should state exactly what is being authorized:

```text
Sign in to CitadelOps Desktop on Nebula-PC
Windows 11 · CitadelOps 2.0.0

This will allow the desktop app to access your Citadel account and current entitlements.
It will not give the website your game password.

[Continue as account@example.com] [Use another account] [Cancel]
```

After success:

- Offer **Open CitadelOps**.
- Show a retry/manual fallback if the deep link fails.
- Let the user choose a different account.
- Name the installation.
- Refresh entitlement without requiring an app restart.

### Recovery

Follow
[OWASP authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
and
[forgot-password](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html)
guidance:

- Use generic response content and comparable timing for existing/nonexistent accounts.
- Use random, single-use, expiring recovery links through a side channel.
- Do not lock an account merely because recovery was requested.
- Do not automatically sign in after a password reset.
- Offer invalidation of existing sessions.
- Notify the owner after recovery.
- Provide backup/recovery codes before they are needed.
- Make support escalation resistant to social engineering.

### Device management

Each installation row should show:

- User-assigned name.
- Operating system.
- App version and update channel.
- First activated and last active.
- Current device marker.
- License slot consumed.
- Health/freshness where available.
- Revoke/free slot action.

Do not call a browser session a device slot. Revocation should say what happens to the local app,
active automation, cached data, and future sign-in.

### Subscription and entitlement states

| State                 | Required message and controls                                                    |
| --------------------- | -------------------------------------------------------------------------------- |
| Trialing              | Days remaining, included features, exact first charge/date, cancel path          |
| Active                | Plan, entitlement, limits, exact renewal amount/date                             |
| Incomplete            | Payment/setup requirement; do not say merely “inactive”                          |
| Past due / grace      | Cause, consequence, grace deadline, update-payment action, access policy         |
| Cancel at period end  | Exact access end date, what remains available, Resume action                     |
| Unpaid                | Explicit read-only/paused behavior and recovery path                             |
| Paused                | Reason, preserved data, resume requirements                                      |
| Canceled              | Access end date, data retention/export, reactivate path                          |
| Expired               | What stopped, what remains viewable/exportable, renewal path                     |
| Revoked               | Reason category, support/appeal path, device/account consequence                 |
| Offline license check | Last validation, grace remaining, retry behavior, safe-mode policy               |
| Server unavailable    | Distinguish from invalid entitlement; keep safe local access where policy allows |
| Refunded              | Financial result and current entitlement                                         |

Never display “License invalid” without cause category, access impact, relevant date, and a remedy.

### Plan and billing page

- Plan name.
- Explicit entitlements and limits.
- Device slots used/available.
- Exact next charge, date, currency, and tax treatment.
- Payment state and payment method.
- Invoices and receipts.
- Renewal setting.
- Update payment, resume, change, or cancel actions.

[Stripe Entitlements](https://docs.stripe.com/billing/entitlements?dashboard-or-api=api) is a useful
model for driving access through explicit feature entitlements rather than inferring everything from
a plan label. [Stripe's customer portal](https://docs.stripe.com/customer-management) demonstrates a
bounded surface for invoices, payment methods, and subscription management; using Stripe is not a
requirement.

### Cancellation

Distinguish:

- **Stop renewal at period end**.
- **End access now**, if the product offers it.

Before confirmation, state:

- Exact access end time and timezone.
- Whether running automations stop immediately or at a safe boundary.
- What definitions, history, and local data are retained.
- Which read/export/diagnostic access remains.
- Whether device slots are released.
- How to resume before the end date.

Send a durable confirmation. Do not create a retention maze. The
[FTC's dark-pattern report](https://www.ftc.gov/reports/bringing-dark-patterns-light) documents
harms from obstructive cancellation and hidden subscription terms.

Regulatory details are time- and region-sensitive. For example, the FTC's 2024 amended Negative
Option Rule was vacated in 2025, and the agency opened a new advance rulemaking process in
March 2026. See the FTC's current
[Negative Option Rule page](https://www.ftc.gov/legal-library/browse/rules/negative-option-rule) and
[March 2026 request for public comment](https://www.ftc.gov/news-events/news/press-releases/2026/03/ftc-seeks-public-comment-response-advance-notice-proposed-rulemaking-regarding-negative-option).
Do not cite a vacated amendment as current law. Counsel should review the final flow and markets.
Transparent terms, informed consent, total-price disclosure, and straightforward cancellation remain
sound product principles.

### Grace and safe degradation

Do not revoke service silently in the middle of a material operation. Product policy should
explicitly define per state:

- Whether new work can start.
- Whether current work may finish safely.
- Whether the user can pause/stop.
- Whether state and activity remain visible.
- Whether data export and support remain available.
- Grace duration and countdown.
- Offline behavior.

The safest default is to preserve visibility, pause unsafe new writes, and provide recovery—not
blank the app.

### Account privacy

Use layered, just-in-time explanation. At diagnostic upload time, state:

- What leaves the machine.
- Purpose.
- Redaction.
- Retention.
- Who can access it.
- Whether it is optional.

Separate required operational data from optional product analytics and crash reporting. Provide
export, deletion, and telemetry controls under **Data and privacy**. The UK's ICO provides practical
[layered privacy-notice guidance](https://ico.org.uk/for-organisations/uk-gdpr-guidance-and-resources/individual-rights/the-right-to-be-informed/what-methods-can-we-use-to-provide-privacy-information/?q=retention).

---

## Onboarding, empty states, and help

### First-run journey

1. Sign in.
2. Connect or discover the game account.
3. Confirm discovered castles.
4. Explain the connection/freshness model.
5. Choose one safe starter automation.
6. Review scope, budget, and stop conditions.
7. Dry-run against current state.
8. Enable it.
9. Show current state, next action, activity, and Pause.

Avoid a long generic carousel before the user has context.
[NN/g's onboarding guidance](https://www.nngroup.com/articles/onboarding-tutorials/) supports
contextual, dismissible, recallable help placed near the actual task.

### Starter content

An empty automation product should not start as an empty canvas. Offer:

- A conservative template.
- A preview based on the user's discovered castles.
- A clear no-premium-currency budget.
- A bounded schedule.
- A test/dry-run.
- A visible path to pause and inspect.

### Empty-state taxonomy

[Carbon distinguishes](https://carbondesignsystem.com/patterns/empty-states-pattern/)
first-use/no-data, no-results, completed/cleared, and unavailable/error states.

| State        | CitadelOps example                  | Content                                              |
| ------------ | ----------------------------------- | ---------------------------------------------------- |
| First use    | No automations                      | Explain value, show starter templates, Create action |
| No attention | Queue is clear                      | “No action needed”; do not imply no data             |
| No history   | No execution yet                    | Explain when history begins and how to run safely    |
| No results   | Search/filters match nothing        | Show active filters, clear/expand controls           |
| Completed    | All selected issues resolved        | Confirm outcome and return path                      |
| Unsupported  | Feature unavailable for environment | Explain why, what remains usable, resolution         |
| Error        | Data could not load                 | Preserve shell/last-known data, retry and details    |

Every empty state should state why it is empty, what normally appears, and the useful next action.

### Loading and partial failure

[Carbon loading guidance](https://carbondesignsystem.com/patterns/loading-pattern/) supports
localized skeletons for structural data expected soon and determinate progress where meaningful.

Rules:

- Preserve the shell and last-known data while refreshing.
- Do not skeletonize stable navigation or dialog chrome.
- Never blank the entire dashboard because one module failed.
- Do not replace a known value with a skeleton during background refresh.
- State last successful refresh and cached status.
- Name the affected connection/module.
- Explain which actions are disabled and why.
- State whether retry is automatic and when.
- Give long operations status, safe cancellation, and failure recovery.

### Command delivery states

Do not label a command successful because it entered a local queue. Where supported, show:

```text
Prepared → Queued → Sending → Acknowledged → Confirmed
                                           ↘ Failed / Timed out / Unknown
```

If the final result is unknown, say unknown and provide reconciliation—not success.

### Contextual help

- Use plain-language inline help for common concepts.
- Use expandable examples for complex settings.
- Keep tooltips brief and noncritical;
  [USWDS advises](https://designsystem.digital.gov/components/tooltip/) against hiding critical
  content in them.
- Link each feature to focused documentation.
- Provide a searchable command/help palette.
- Keep protocol IDs and advanced tuning behind a Developer boundary.

---

## Color theory and the proposed color system

### What color must do

Color in CitadelOps has five separate jobs:

1. **Perceptual structure** — separate canvas, surfaces, controls, text, and focus.
2. **Brand recognition** — provide an ownable, consistent accent.
3. **Semantic state** — success, information, waiting, warning, danger, disabled, stale.
4. **Interaction** — selected, hover, pressed, focus, drag target.
5. **Data encoding** — ordered, divergent, or categorical comparison.

One hue should not perform all five. The current green acts as brand, primary action, active state,
and success, which makes those meanings difficult to distinguish.

### Color psychology: useful but limited

Claims such as “blue means trust” or “green means success” are not universal laws. Effects depend on
context, culture, contrast, surrounding colors, learned conventions, and task. The academic review
[Color Psychology: Effects of Perceiving Color on Psychological Functioning in Humans](https://pubmed.ncbi.nlm.nih.gov/23808916/)
supports treating color effects as context-dependent rather than deterministic.

Therefore:

- Choose colors for differentiation, accessibility, and product positioning first.
- Validate brand associations with target users.
- Never infer safety or urgency from hue alone.
- Avoid “psychology” claims that are not tied to the actual interface context.

### Perceptual color construction

Use OKLCH to construct ramps because lightness and chroma are more perceptually interpretable than
HSL. [CSS Color 4](https://www.w3.org/TR/css-color-4/) defines OKLab/OKLCH support. The 2025 Design
Tokens Color Module also supports modern color spaces in a portable token model:
[DTCG Color Module](https://www.w3.org/community/reports/design-tokens/CG-FINAL-color-20251028/).

This does not eliminate testing:

- Validate rendered sRGB and Display P3 behavior.
- Check gamut mapping.
- Check text/non-text contrast for every real pair.
- Test transparency over every possible background.
- Test light, dark, high contrast, and forced colors.
- Check color-vision variants and grayscale.

### Recommended identity territory: Command Violet

Three territories were considered:

| Territory          | Description                                                            | Advantage                                                        | Risk                                  |
| ------------------ | ---------------------------------------------------------------------- | ---------------------------------------------------------------- | ------------------------------------- |
| Bastion Verdigris  | Stone/slate plus desaturated teal/verdigris                            | Continuity with current green                                    | Brand can still collide with success  |
| **Command Violet** | Navy/graphite plus royal violet action and restrained cyan information | Strong separation from success and a promising ownable direction | Must avoid generic gaming neon        |
| Forge Copper       | Ink plus copper action and cool cyan information                       | Warm, game-adjacent, crafted                                     | Copper can collide with warning amber |

**Command Violet is the strongest current concept hypothesis**, not a proven category distinction.
It separates brand action from semantic success and can remain composed rather than neon when used
sparingly. A competitor color audit and user testing must confirm or reject it.

### Proposed dark palette

This is a contrast-calculated starting palette, not a final token release. Only the foreground and
background pairs with ratios shown below have been calculated; the system still requires rendered
component and compositing tests.

| Role            |     Value | Intended use                          | Contrast note                                      |
| --------------- | --------: | ------------------------------------- | -------------------------------------------------- |
| Canvas          | `#07111F` | App background                        | Strong deep neutral                                |
| Surface         | `#0D1B2A` | Main panels                           | —                                                  |
| Raised surface  | `#13263A` | Inspector/dialog                      | —                                                  |
| Subtle border   | `#29425D` | Nonessential grouping/separation      | 1.68:1 on surface; never the sole control boundary |
| Control border  | `#526D89` | Inputs, controls, meaningful graphics | About 3.24:1 on surface                            |
| Text strong     | `#F4F8FC` | Headings/primary values               | 17.75:1 on canvas                                  |
| Text default    | `#D6E1EC` | Body/control text                     | 13.12:1 on surface                                 |
| Text muted      | `#9DB0C3` | Secondary metadata                    | 7.81:1 on surface                                  |
| Brand/action    | `#9B8CFF` | Primary action, selected focus        | 6.29:1 on surface                                  |
| Brand fill text | `#0B1024` | Text on bright brand fill             | 6.81:1 on brand                                    |
| Success         | `#4CD58A` | Confirmed healthy/success             | 9.27:1 on surface                                  |
| Warning         | `#F4BE53` | Waiting risk/attention                | 10.22:1 on surface                                 |
| Danger          | `#FF6B76` | Failed/destructive/critical           | 6.31:1 on surface                                  |
| Information     | `#48C6E8` | Informational/correlation             | 8.71:1 on surface                                  |
| Info fill text  | `#061723` | Text on cyan fill                     | 9.11:1 on information                              |
| Inactive        | `#8292A6` | Disabled/off/unknown                  | Pair with text/icon, not low-opacity alone         |

### Proposed light palette

| Role           |     Value | Intended use                          | Contrast note                                    |
| -------------- | --------: | ------------------------------------- | ------------------------------------------------ |
| Canvas         | `#F4F7FA` | App background                        | —                                                |
| Surface        | `#FFFFFF` | Main panels                           | —                                                |
| Subtle surface | `#EAF0F5` | Groups/selected background            | —                                                |
| Subtle border  | `#C9D5E1` | Nonessential grouping/separation      | 1.49:1 on white; never the sole control boundary |
| Control border | `#8292A4` | Inputs, controls, meaningful graphics | About 3.18:1 on white                            |
| Text strong    | `#102033` | Primary text                          | 16.45:1 on white                                 |
| Text muted     | `#52687D` | Secondary text                        | 5.78:1 on white                                  |
| Brand/action   | `#5A4BCB` | Primary action/links                  | 6.36:1 on white and with white text              |
| Success        | `#147A46` | Success text/icon                     | 5.38:1 on white                                  |
| Warning        | `#8B5800` | Warning text/icon                     | 6.01:1 on white                                  |
| Danger         | `#B42338` | Danger text/icon                      | 6.49:1 on white                                  |
| Information    | `#007B99` | Informational text/icon               | 4.89:1 on white                                  |

Contrast values above use WCAG 2.x relative-luminance calculations and should be revalidated in the
actual theme, fonts, sizes, states, and composited backgrounds. Do not round a failing ratio up.

### Usage proportions

As a directional composition rule:

- 80–90% neutral canvas/surfaces/text.
- 5–10% brand/action and selection.
- Less than 5% combined semantic status in a healthy screen.
- Critical color appears only where a critical consequence exists.

This is not a rigid mathematical rule. It protects operational signal from decorative chroma.

### Semantic rules

- **Brand violet:** primary action, selected navigation, user-controlled emphasis.
- **Green:** confirmed success/healthy only.
- **Amber:** waiting with consequence, caution, near-term attention.
- **Red/coral:** failure, destructive action, critical risk.
- **Cyan:** neutral information, connection detail, correlation.
- **Gray/slate:** disabled, inactive, unknown, stale supporting state.
- **Purple is never also “AI.”** AI identity should be indicated by actor label/icon; reusing brand
  purple avoids inventing a magical semantic color.
- Every semantic use includes a word and icon/shape.

### Enabled versus running versus complete

These often-confused states should look different:

| State              | Treatment                                                      |
| ------------------ | -------------------------------------------------------------- |
| Enabled            | Neutral/violet selection indicator plus “Enabled”              |
| Waiting            | Clock icon, neutral or amber only if attention is needed       |
| Running            | Stable progress/step label; no ambient pulsing required        |
| Confirmed complete | Green check and receipt                                        |
| Disabled           | Neutral gray, still legible                                    |
| Failed             | Red error icon and cause                                       |
| Stale              | Neutral dashed/clock treatment plus timestamp; not generic red |

### Accessibility rules

[WCAG 2.2](https://www.w3.org/TR/WCAG22/) requires, at AA:

- 4.5:1 for normal text.
- 3:1 for large text.
- 3:1 for meaningful non-text controls/graphics against adjacent colors.
- No information conveyed by color alone.

See [Use of Color](https://www.w3.org/WAI/WCAG22/Understanding/use-of-color) and
[Non-text Contrast](https://www.w3.org/WAI/WCAG22/understanding/non-text-contrast.html).

Support:

- System, Light, and Dark theme choices.
- `forced-colors` adaptation.
- `prefers-contrast` where supported.
- Reduced transparency setting or media query.
- Colorblind-safe redundant cues.
- User-configurable data-viz palette where useful.

The Xbox Accessibility Guidelines emphasize redundant channels and warn that simulation is not a
substitute for testing with players:
[XAG 103](https://learn.microsoft.com/en-us/xbox/accessibility/xbox-accessibility-guidelines/103).

### Glass and transparency

Use glass only for a small number of transient or shell-level layers.
[Apple's Materials guidance](https://developer.apple.com/design/human-interface-guidelines/materials)
treats glass as a functional layer and recommends thicker/more opaque treatment when legibility
requires it. [web.dev](https://web.dev/articles/backdrop-filter?hl=en) notes that backdrop filtering
can harm performance and must be tested.

CitadelOps rules:

- Main reading and scrolling surfaces are mostly opaque.
- No blur behind dense text, tables, or logs.
- Do not stack multiple translucent layers.
- High-contrast and reduced-transparency modes remove glass.
- Focus and state boundaries never depend on blur.

---

## Brand strategy and identity

### Brand positioning

**Category:** Game-operations automation and decision-support console.  
**Audience:** Players who want leverage and control without surrendering understanding.  
**Promise:** Calm command over complex game operations.  
**Reason to believe:** Deterministic plans, visible scope, resource guardrails, safe pause, traces,
and receipts.

### Brand personality

Primary traits:

- Competent.
- Composed.
- Transparent.
- Protective.
- Precise.
- Player-respecting.

Secondary expression:

- Tactical, not militaristic.
- Capable, not aggressive.
- Modern, not cyberpunk.
- Game-aware, not fantasy-skeuomorphic.
- Confident, not celebratory or boastful.

Jennifer Aaker's research on
[brand personality dimensions](https://www.gsb.stanford.edu/faculty-research/publications/dimensions-brand-personality)
provides a useful vocabulary. CitadelOps should emphasize Competence and Sincerity, with only
restrained Ruggedness/Excitement.

### Brand behaviors

The brand is proven by behavior:

- Always say what automation is doing.
- Explain why it is waiting or acting.
- State what comes next and when.
- Mark live, stale, cached, and unknown data.
- Show scope and resource consequence.
- Make stop/reversal discoverable.
- Admit uncertainty and upstream responsibility.
- Use exact language over vague celebration.

Prefer:

> Upgrade queued — starts when construction slot 2 is free in about 18 minutes.

Avoid:

> Automation successful! Your empire is unstoppable.

### Brand architecture across surfaces

Invariant:

- Cleared name and wordmark.
- Symbol family.
- Brand violet and neutral family.
- Typography system.
- Icon geometry.
- Tone.
- Motion curve.
- Accessibility standard.

Variable:

- Public site: more expressive type, art, and product narrative.
- Account portal: quiet, explicit, trust-heavy.
- Desktop app: high information density with restrained ornament.

### Naming and trademark gate

Before committing to the name:

1. Define goods/services and launch regions.
2. Search exact and similar word marks, phonetics, spelling variants, translations, and meanings.
3. Search federal, state, common-law, company, app-store, domain, social, and international records.
4. Search related design marks once symbols exist.
5. Assess relatedness of services and likely commercial impression.
6. Obtain qualified legal advice.
7. Record the decision and acceptable risk.
8. Only then finalize wordmark, domains, social handles, store listings, and public launch.

See the USPTO's
[federal search guidance](https://www.uspto.gov/trademarks/search/federal-trademark-searching),
[likelihood-of-confusion guidance](https://www.uspto.gov/trademarks/search/likelihood-confusion),
and [strong-trademark guidance](https://www.uspto.gov/trademarks/basics/strong-trademarks).

### Current logo assessment

The current mark combines:

- An outlined hexagon.
- Four growth bars.
- An ascending trend line.
- White and emerald strokes.

It communicates “analytics/operations” but is not strongly ownable and loses clarity at small sizes.
Hexagon + chart + security/operations associations are crowded in software, crypto, and
infrastructure categories.

Avoid adding more generic symbols such as shield + castle + gear + robot + chart.

### Recommended icon exploration

Explore three black-silhouette territories after name clearance:

1. **Bastion Loop** — a keep/arch constructed from one continuous automation path. Best conceptual
   fusion of citadel and orchestration.
2. **Citadel Monogram** — a custom C made from three or four bastion segments around a central
   command core. Strongest small-icon potential; test that it does not read as a gear.
3. **Beacon Keep** — a central keep with two restrained coordination arcs/nodes. Communicates
   vigilance and routing without a shield.

The first concept screen set should use a reversible **Citadel Monogram / command core** hypothesis
because it remains legible as a favicon and can be drawn in one color.

### Icon process

1. As a proposed exploration guardrail, sketch at least 20 black silhouettes before polishing one.
2. Remove color and wordmark; test recognition.
3. Compare against adjacent gaming, automation, security, and IT marks.
4. Test at 16, 24, 32, and 48 pixels.
5. Use slight blur and five-second recall tests.
6. Test reversed, monochrome, grayscale, and forced colors.
7. Test circle, square, squircle, and maskable crops.
8. Conduct legal/design-mark searches.
9. Create optical redraws for micro sizes rather than naïve scaling.

Required asset family:

- Master symbol.
- Horizontal and stacked lockups.
- One-color and reversed marks.
- 16-pixel micro icon.
- 24/32/48-pixel UI variants.
- 256/512/1024 app art.
- Maskable PWA asset.
- Monochrome tray/notification icon.
- Favicon.

See
[Apple app-icon guidance](https://developer.apple.com/design/human-interface-guidelines/app-icons),
[Windows app-icon design](https://learn.microsoft.com/en-us/windows/apps/design/iconography/app-icon-design),
and the [Web App Manifest maskable icon rules](https://www.w3.org/TR/appmanifest/).

### Wordmark direction

- Use a restrained custom wordmark only after the name decision.
- Prefer engineered, open counters and strong small-size spacing over fantasy serifs or stencil
  type.
- A single subtle architectural cut or bastion detail is enough; do not modify every letter.
- Maintain a plain-text fallback and never require the wordmark to identify the product in UI.
- Create horizontal, stacked, symbol-only, monochrome, and small-size lockups.
- Define clear space and minimum size from optical tests, not arbitrary percentages.

### Type-family shortlist

These are evaluation candidates, not a licensing conclusion:

| Candidate                  | Character                                            | Best use                                      | Check before selection                                |
| -------------------------- | ---------------------------------------------------- | --------------------------------------------- | ----------------------------------------------------- |
| Inter Variable             | Neutral, compact, excellent UI numerals              | Desktop and account UI                        | Self-hosting, language coverage, app rendering        |
| IBM Plex Sans Variable     | Technical but human; distinctive                     | Product UI and documentation                  | Width at compact density, bundle subsets              |
| Atkinson Hyperlegible Next | Character differentiation and accessibility emphasis | Accessibility-oriented alternative or UI      | Brand fit, density, language coverage                 |
| Source Serif 4             | Editorial authority without fantasy styling          | Optional public-site long-form/display accent | Use sparingly; loading and licensing assets           |
| Native system stack        | Fast and platform-authentic                          | Prototype or performance-first app            | Cross-platform metric drift and brand distinctiveness |

Confirm the exact font files, license, redistribution, language subsets, hinting, variable-axis
support, and Windows rendering before commitment. The concept prototype can use a native/system
stack to avoid implying a final type-license decision.

### Imagery and illustration

- Use real product screenshots with truthful, readable states.
- Use abstract route, bastion, queue, and coordination motifs for brand illustration.
- Prefer restrained texture and geometry to generic fantasy castles, glowing AI brains, robots, or
  stock “cybersecurity” imagery.
- Separate operational color from illustrative color; illustration must not look like live status.
- Show humans only when there is a real user story, not as generic stock trust decoration.
- Establish screenshot framing, annotation, redaction, theme, and demo-data rules.
- Confirm rights before using game art, names, icons, screenshots, or other third-party assets in
  public marketing. Product availability does not imply marketing rights.
- Provide alt text for informative imagery and empty alt for decorative texture.

### Competitor and category audit before final identity

Build a comparison grid across:

- Game companion and automation tools.
- Workflow/automation products.
- Observability/operations consoles.
- Security and IT “citadel/command” brands.
- Game launchers and utilities.

Compare word marks, silhouettes, dominant hues, gradients, shield/hexagon/castle/gear/robot/chart
metaphors, typography, app-store thumbnails, 16-pixel icons, and tone. The goal is to prove both
legal and perceptual distance, not merely produce a preferred mood board.

---

## Typography

### Productive and expressive scales

[Carbon distinguishes](https://carbondesignsystem.com/elements/typography/type-sets/) a productive
14-pixel base and an expressive 16-pixel base. Recommended CitadelOps scale:

|     Size | Use                                                    |
| -------: | ------------------------------------------------------ |
|    12 px | Rare nonessential metadata only                        |
|    14 px | Dense dashboard controls, table cells, supporting text |
|    16 px | Primary body, forms, status, comfortable density       |
|    20 px | Section heading                                        |
| 24–32 px | Page title or important metric                         |
| 40–64 px | Public-site display only                               |

Rules:

- One UI family; at most one restrained display family for marketing.
- Prefer a bundled variable font or reliable system stack.
- Use tabular numerals for resources, timers, coordinates, and timestamps.
- Right-align comparable quantities.
- Use monospace only for raw payloads, IDs, and code—not the entire product.
- Use weight, size, spacing, color, and position together; do not rely on weight alone.
- Keep long reading lines around 65–75 characters.
- Avoid all-caps for long labels; allow deliberate tracking for short eyebrow labels.
- Remove the global letter-spacing override.

The interface must survive user text-spacing overrides: line height 1.5×, paragraph spacing 2×,
letter spacing 0.12em, and word spacing 0.16em per
[WCAG Text Spacing](https://www.w3.org/WAI/WCAG22/Understanding/text-spacing.html).

### Tone and microcopy

- Use active, concrete verbs: Connect, Pause, Resume, Review, Retry safe step.
- Name the object: Pause all automations, not Lock Bot.
- Name consequence in confirmations: Spend 12,500 silver, not Confirm.
- Avoid anthropomorphism for routine system action.
- Use “about” for estimates and exact timestamps in detail.
- Use “Unknown” when the system genuinely does not know.
- Distinguish “Not yet confirmed” from “Failed.”

---

## Layout, density, and responsive behavior

### Grid

- 8-pixel macro grid with a 4-pixel micro unit.
- Content-driven breakpoints rather than device labels alone.
- Up to a 16-column wide desktop grid.
- Stable alignment for status, scope, time, and actions.
- Use container queries for reusable cards and panes where appropriate; see
  [MDN container queries](https://developer.mozilla.org/en-US/docs/Web/CSS/Guides/Containment/Container_queries).

### Shell

```text
Wide:    navigation | main task canvas | optional context inspector
Medium:  icon/compact navigation | main canvas | inspector as drawer
Narrow:  one task surface at a time | detail as route or bottom sheet
```

Retain selection, scroll, filters, and draft state when a pane collapses.

### Density modes

- **Comfortable:** onboarding, forms, account portal, touch-capable use.
- **Compact:** tables, queues, logs, expert operations.

Density changes padding and row height—not font legibility, target availability, or information
semantics.

### Avoid card soup

Use cards only for independently selectable, reorderable, or self-contained objects. Use:

- Page sections.
- Banded groups.
- Dividers.
- Structured lists.
- Tables.
- Split views and inspectors.

Recommended radius scale:

- 4 px: small controls and table selections.
- 8 px: standard controls and compact panels.
- 12 px: primary surfaces and dialogs.
- 16+ px: rare marketing or major illustration frames.
- Full circle/pill: only where semantics require it.

Use two or three meaningful elevation levels. Shadows and blur do not create hierarchy when applied
everywhere.

### Hierarchy order

1. State now.
2. Attention required.
3. What happens next.
4. Primary action.
5. Supporting metrics.
6. Raw evidence.

---

## Desktop-native behavior

The product is rendered with web technology but behaves as a long-running desktop application.

### Window and display behavior

- Restore window size, position, last meaningful route, density, and inspector state safely.
- If a monitor is removed or its scale changes, move the window back into a visible work area.
- Test 100%, 125%, 150%, 200%, and mixed-DPI multi-monitor setups.
- Avoid one-pixel strokes or bitmap assets that blur under scaling.
- Preserve usable minimum width; do not prevent the user from resizing narrower than the ideal.
- Offer a reset-layout action.
- Do not persist a dialog that reopens offscreen.

### Tray and background operation

Product policy must decide whether closing the window exits, minimizes, or leaves automation
running. Never rely on convention alone.

- First close with active automation explains the chosen behavior and lets the user remember it.
- Tray menu exposes Open, connection summary, Pause all automations, and Quit.
- Quit states what happens to local service, game connection, and current operations.
- Tray icon has a monochrome OS-appropriate form and does not encode critical state by color alone.
- Background behavior remains visible in account/device and diagnostics history.

### OS notifications

- Off by default for routine completions.
- User-configurable by severity, feature, castle, quiet hours, and channel.
- Never include secret payloads or sensitive account/game data on a lock screen.
- Notification action deep-links to a durable scoped view.
- Dismissing an OS notification is not acknowledging or resolving the underlying issue.
- Respect OS focus/do-not-disturb controls.

### Updates

- Show installed version, available version, channel, release notes, and publisher.
- Download in the background with progress and integrity verification.
- Prompt to restart at a safe time and preserve drafts/workspace state.
- Routine updates do not block every launch.
- Security-critical or protocol-incompatible updates may block only with a clear reason and a safe
  path to preserve/export work.
- Define what happens to active automation during update and restart.

### Windows accessibility and system integration

- Map correctly to Windows high-contrast/forced colors.
- Expose accessible window, dialog, control, and notification names through the host stack.
- Support system theme and text scaling.
- Use standard clipboard, file picker, save/export, and browser-auth handoff patterns.
- Test keyboard access without relying on browser-only conventions that conflict with desktop
  expectations.

---

## Motion

Motion communicates causality and spatial change. It should not advertise that the app is “alive.”

Proposed timing guardrails to validate in the prototype:

| Interaction            |            Duration |
| ---------------------- | ------------------: |
| Button/toggle feedback |           80–120 ms |
| Small reveal/expansion |          120–180 ms |
| Drawer/inspector       |          180–240 ms |
| Toast/system message   |          200–280 ms |
| Major page transition  | No more than 300 ms |

[Carbon motion](https://carbondesignsystem.com/elements/motion/overview/) and
[Atlassian motion](https://atlassian.design/foundations/motion) offer comparable productive timing
ranges.

Rules:

- No bounce/elastic novelty in operational flows.
- No ambient pulse for healthy/running state.
- Use a stable icon, label, time, and progress measure.
- Prefer transform and opacity.
- Avoid animating layout, large blur, and expensive paint.
- Respect `prefers-reduced-motion` and offer an in-product reduction option.
- Avoid auto-updating motion that cannot be paused.

---

## Iconography

The current Lucide foundation is suitable if governed consistently.

Rules:

- One navigation meaning per silhouette.
- Keep labels visible for primary navigation and unfamiliar actions.
- Tooltips supplement rather than replace labels.
- Decorative icons are hidden from assistive technology.
- Icon-only controls receive concise accessible names.
- Use one stroke grammar and optical size system.
- 16/20 pixels for inline use; 24/32 for larger actions.
- Do not reuse shields, activity waves, lightning, or stars for unrelated meanings.
- Actor icons identify User, Automation, AI, Citadel, and Game—but the word remains available.

The [WAI-ARIA naming guidance](https://www.w3.org/WAI/ARIA/apg/practices/names-and-descriptions/)
should inform accessible labels; the visual shape does not substitute for a programmatic name.

---

## Data visualization

### Chart selection

| Question                            | Preferred form                              |
| ----------------------------------- | ------------------------------------------- |
| How did one value change over time? | Line with direct labels and relevant events |
| How do categories compare?          | Sorted bar chart                            |
| How close are we to a threshold?    | Linear progress/bullet with threshold       |
| When will queued events occur?      | Timeline                                    |
| What is exact current state?        | Number + timestamp + status, often no chart |
| How is a total composed?            | Stacked bar only if parts must be compared  |

Avoid a dashboard full of donuts, gauges, and decorative sparklines.

### Color scales

- Sequential for ordered low-to-high values.
- Diverging only around a meaningful midpoint/target.
- Qualitative for nominal groups.
- Avoid rainbow scales.
- Use direct labels, shapes, line styles, or patterns in addition to hue.

[ColorBrewer's scheme guidance](https://colorbrewer2.org/learnmore/schemes_full.html) explains these
categories.
[USWDS data-visualization guidance](https://designsystem.digital.gov/components/data-visualizations/)
recommends one central idea, text summaries, and alternatives to color-only encoding.

### Every KPI answers

- What is the value?
- Compared with what?
- As of when and how fresh?
- What action follows?

### Chart accessibility

- Do not require hover.
- Provide a short intended-message summary.
- Provide accessible table/detail for underlying values where useful.
- Identify chart scale and units.
- Give complex charts a long description of trends and notable points, following
  [W3C complex-image guidance](https://www.w3.org/WAI/tutorials/images/complex/).

---

## Accessibility specification

### Baseline

Target **WCAG 2.2 AA** for public website, account portal, and desktop UI, with selected AAA
practices where they materially improve an operations product. Use the
[Xbox Accessibility Guidelines](https://learn.microsoft.com/en-us/xbox/accessibility/guidelines) as
additional game-adjacent guidance, not as a legal compliance standard.

Accessibility belongs in design-system component contracts and acceptance criteria.

### Keyboard and focus

- Every task is operable without a mouse.
- Focus order follows visual/task order.
- Current navigation uses native links and `aria-current`.
- Composite widgets use established ARIA patterns.
- Escape closes dismissible overlays and returns focus.
- Dialogs set initial focus, trap focus, expose title/description, and restore focus.
- Drawers, logs, sticky headers, and toasts do not obscure the focused element.
- Shortcuts are discoverable, remappable where appropriate, and do not conflict with text entry.
- Drag-and-drop has click/keyboard alternatives.

[XAG 112](https://learn.microsoft.com/en-us/xbox/accessibility/xbox-accessibility-guidelines/112)
emphasizes consistent navigation, predictable focus, and digital/keyboard input.
[WCAG Focus Not Obscured](https://www.w3.org/WAI/WCAG22/Understanding/focus-not-obscured-minimum.html)
is especially relevant to the proposed inspector and log dock.

### Focus appearance

- Use a dual-color or otherwise background-independent focus ring.
- Target at least a visible 2-pixel perimeter-equivalent and 3:1 change.
- Do not remove outlines without a replacement.
- Selected and focused remain distinguishable.

See [WCAG Focus Appearance](https://www.w3.org/WAI/WCAG22/Understanding/focus-appearance.html).

### Targets

- Meet the WCAG 2.2 AA 24-by-24-pixel minimum or spacing exception.
- Prefer 44-by-44 pixels for primary, frequent, destructive, and touch-capable controls.
- Preserve adequate spacing between icon-only actions.
- Do not shrink essential controls in Compact density below accessible operation.

See [Target Size Minimum](https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html) and
[Target Size Enhanced](https://www.w3.org/WAI/WCAG22/Understanding/target-size-enhanced.html).

### Text, zoom, and reflow

- Support 200% text resize without loss.
- Support 320 CSS-pixel reflow / 400% zoom without two-dimensional scrolling except inherently 2D
  content such as wide data tables or maps.
- Provide a responsive alternative or scoped horizontal scroll for inherently 2D content.
- Avoid essential 10–11-pixel labels.
- Do not truncate the only accessible version of critical state.
- Test user text spacing and long translations.

See [Resize Text](https://www.w3.org/WAI/WCAG22/Understanding/resize-text.html) and
[Reflow](https://www.w3.org/WAI/WCAG22/Understanding/reflow.html).

### Status and live updates

- Programmatically expose meaningful busy, progress, success, and failure changes without moving
  focus.
- Do not announce every incoming log event.
- Announce a transition back to available when useful.
- Use `role="log"` only for carefully scoped sequential updates.
- Let users pause, stop, hide, or reduce frequency of auto-updating parallel information.
- Countdowns must remain understandable without animation.

See [WCAG Status Messages](https://www.w3.org/WAI/WCAG22/Understanding/status-messages.html) and
[Pause, Stop, Hide](https://www.w3.org/WAI/WCAG22/Understanding/pause-stop-hide).

### Hover and tooltip content

- Essential content is visible or reachable without hover.
- Hover/focus overlays are dismissible, hoverable, and persistent while needed.
- Native `title` is not the only explanation for automation status.

See
[Content on Hover or Focus](https://www.w3.org/WAI/WCAG22/Understanding/content-on-hover-or-focus.html).

### Motion, flashing, and transparency

- Respect reduced motion.
- Avoid flashing and rapid luminance changes.
- Provide stable alternatives for animated progress.
- Avoid parallax and motion triggered solely by pointer movement.
- Support reduced transparency/high contrast.
- Do not use glow as the only focus or state cue.

### Forms and errors

- Persistent labels, not placeholder-only fields.
- Programmatic descriptions and constraints.
- Error summary plus inline error.
- Preserve all valid input.
- Explain problem and fix.
- Focus the summary only when appropriate and retain a route to the field.
- Confirmation button names the consequence.
- Password managers and paste are allowed.

[GOV.UK validation guidance](https://design-system.service.gov.uk/patterns/validation/) is a strong
reference for summary plus inline placement and input preservation.

### Tables and charts

- Correct header associations and captions.
- Sort state exposed programmatically.
- Selection state named.
- Keyboard access to row actions.
- Units included in headers or accessible text.
- Text summary for chart meaning.
- Accessible underlying values where useful.
- Do not use color-only series distinctions.

### Accessibility test matrix

- Keyboard only.
- Screen reader on supported OS/browser combinations.
- 200% text.
- 400% zoom/320 CSS pixels.
- User text-spacing overrides.
- Light, dark, System.
- Forced colors and high contrast.
- Reduced motion and reduced transparency.
- Protanopia, deuteranopia, tritanopia, grayscale.
- Comfortable and Compact density.
- First use, empty, loading, stale, disconnected, partial failure, permission, license, error, and
  recovery states.
- Long localized strings and right-to-left samples.

---

## Performance and resilience specification

### Two performance contexts

CitadelOps has both:

1. A public web/account experience measured like a normal website.
2. A long-running live desktop console driven by sustained events.

The second must remain stable after hours, not only load quickly once.

### Experience targets

Use current Core Web Vitals as public-site targets at the 75th percentile:

- LCP no more than 2.5 seconds.
- INP no more than 200 milliseconds.
- CLS no more than 0.1.

See
[Core Web Vitals thresholds](https://web.dev/articles/defining-core-web-vitals-thresholds?hl=en),
[LCP](https://web.dev/articles/lcp), [INP](https://web.dev/articles/inp), and
[CLS](https://web.dev/articles/cls).

Desktop/app targets:

| Metric                             |                      Directional budget |
| ---------------------------------- | --------------------------------------: |
| Visual acknowledgment of input     |                            Under 100 ms |
| Common route becomes useful        |      Under 1 s after shell/cached state |
| Frame cadence under active updates |        60 fps target; no sustained jank |
| Layout shift during live updates   |           None in the primary task area |
| Log open/search/scroll             |          Responsive at retained maximum |
| State update                       | Only affected domains/components render |
| Hidden workspaces                  |        No unnecessary polling/rendering |
| Long-running memory                |  Bounded by explicit retention policies |

[RAIL guidance](https://web.dev/articles/rail) uses 100 milliseconds as the immediate-response
window and a 16.7-millisecond frame at 60 Hz, leaving roughly 10 milliseconds for application work
after browser overhead.

### Architecture recommendations

#### Loading and code

- Route-level lazy loading.
- Lazy-load large modals/editors and heavy analytics.
- Preload the next likely workspace only when idle and useful.
- Split marketing/account/desktop bundles if hosted together.
- Keep startup shell independent from large game catalogs.
- Measure compressed and parsed/executed cost, not only raw bytes.

#### State

- Domain-scoped stores/subscriptions or selectors.
- Avoid one broad context invalidating the entire app.
- Prefer incremental state events to broad full-state refetches where safe.
- Normalize by stable IDs.
- Batch high-frequency WebSocket updates.
- Assign update priority; urgent state before background metrics.
- Preserve the state revision used by plans/executions.

#### Polling and lifecycle

- Suspend view-specific polling while inactive.
- Prefer server events when authoritative.
- Back off during hidden/background state.
- Use one shared scheduler rather than independent component timers.
- Expose the source and cadence of stale data.

#### Lists and logs

- Virtualize tables, media grids, picker lists, activity, and logs.
- Preserve scroll anchor.
- Do not pretty-print every JSON payload eagerly.
- Defer search/filtering with responsive input.
- Move expensive parsing/search to a worker when measurements justify it.
- Use bounded retention and prune receipts/operations.

The current TanStack Virtual dependency is appropriate. General rationale is described in
[web.dev's long-list virtualization guidance](https://web.dev/articles/virtualize-long-lists-react-window).

#### Images and game data

- Lazy-load by visible workspace and item.
- Reserve dimensions to prevent layout shift.
- Generate appropriate sizes/formats.
- Deduplicate official game assets and avoid packaging unnecessary copies.
- Use a runtime cache consistent with the architecture's stated data ownership.
- Decode off the critical path where possible.
- Provide stable fallback and missing-asset behavior.

#### CSS and visual effects

- Remove full-scroll backdrop filters.
- Avoid stacked blur/shadow.
- Animate transform/opacity rather than layout.
- Use `content-visibility` only after measuring and testing focus/find/accessibility behavior; see
  [web.dev](https://web.dev/articles/content-visibility).
- Keep DOM size bounded; [web.dev explains](https://web.dev/articles/dom-size-and-interactivity) its
  effect on style, layout, and interaction latency.

### Resilience model

Partial failure is expected. The UI must independently represent:

- Account/entitlement service.
- Local desktop process.
- Managed browser.
- Game WebSocket.
- Game authentication/session.
- State normalization/catalog data.
- Update service.

Rules:

- Preserve healthy scopes.
- Preserve last-known data with time and cached label.
- Disable only actions whose preconditions are unsafe.
- Explain automatic retry and next time.
- Allow safe cancellation of queued work.
- Reconcile unknown command outcomes before repeating.
- Never treat a license-service outage as proof that entitlement is invalid.

### Performance instrumentation

- Public-site Web Vitals by route/device/region.
- App route-load timing.
- State payload size and update frequency.
- React render counts/duration by major workspace.
- Main-thread long tasks.
- Frame drops with log dock open.
- Image/cache memory.
- Retained log, activity, and receipt counts.
- End-to-end intent timing: planned, queued, sent, acknowledged, confirmed.
- Long-session memory sampling.

Performance budgets should fail design review when exceeded, even if a visual effect looks
attractive.

---

## Design-system architecture

The first stable
[Design Tokens Community Group specification, 2025.10](https://www.designtokens.org/) provides a
vendor-neutral format for sharing design decisions. It is useful as a storage/interchange format;
naming and governance still require product-specific design.

### Three token layers

```text
Primitive
  color.neutral.950
  color.violet.400
  space.4
  radius.2

Semantic
  surface.canvas
  surface.raised
  text.primary
  action.primary
  status.warning
  focus.ring.outer

Component
  button.primary.background.default
  automation.row.blocked.icon
  log.row.error.border
```

Components consume semantic or component tokens, not raw hex values.

### Token domains

- Color.
- Typography.
- Spacing.
- Radius.
- Border.
- Elevation.
- Motion.
- Density.
- Z-index/layers.
- Breakpoints/container states.
- Data-visualization schemes.
- Opacity and transparency.
- Icon size/stroke.

### Theme mapping

- Light, Dark, System.
- High-contrast/forced-color behavior.
- Reduced-transparency mapping.
- Comfortable and Compact density.
- Public expressive versus product productive typography.

Theme changes semantic roles; components do not switch raw colors independently.

### Component contract

Every shared component documents:

- Purpose.
- When to use and not use.
- Anatomy.
- Content rules.
- Default, hover, pressed, focus, disabled, selected, loading, error states as relevant.
- Keyboard behavior.
- Accessible name/description/status.
- Responsive behavior.
- Theme and forced-color behavior.
- Density behavior.
- Localization/RTL behavior.
- Performance caveats.
- Testing requirements.

### Foundational component priority

1. App shell and routed navigation.
2. Button and icon button.
3. Link.
4. Status indicator.
5. Alert/banner/toast.
6. Input, select/combobox, checkbox, radio, switch, segmented control.
7. Dialog, drawer, popover, tooltip.
8. Table/list/virtual row.
9. Empty/loading/error/stale state.
10. Activity item and operation step.
11. Automation row/card and attention item.
12. Timeline/progress/data-viz primitives.

Fixing these before redesigning every view prevents inconsistent one-off accessibility and visual
behavior.

### Governance

- Named system owner(s).
- Contribution and review criteria.
- Semantic naming rules.
- Versioning and deprecation.
- Accessibility acceptance.
- Visual regression examples across themes/density/states.
- Usage analytics where appropriate.
- Migration guidance.
- Decision log for exceptions.

Numeric primitive scales should leave gaps for future insertion rather than encoding component names
into primitives.

---

## Localization and internationalization

- Use CSS logical properties.
- Use `Intl.DateTimeFormat`, `Intl.NumberFormat`, and `Intl.RelativeTimeFormat`.
- Show time zone when absolute incident/billing time matters.
- Use CLDR plural categories rather than `item(s)`; see
  [CLDR plural rules](https://cldr.unicode.org/index/cldr-spec/plural-rules).
- Pseudolocalize at least +40%; short labels may expand 200–400%. Microsoft's
  [pseudolocalization guidance](https://learn.microsoft.com/en-us/globalization/methodology/pseudolocalization)
  explains why.
- Do not build layouts around fixed English button widths.
- Keep raw protocol payloads literal and LTR while localizing surrounding labels.
- Isolate IDs, coordinates, and raw strings with `bdi`/direction metadata in bidirectional text; see
  [W3C strings and bidi](https://www.w3.org/international/articles/strings-and-bidi/).
- Avoid concatenated sentences.
- Localize units, number separators, relative time, and accessible names.
- Test navigation, status chips, tables, dialogs, and notifications under expansion.

---

## Content model and vocabulary

### Canonical terms

| Concept                     | Preferred term         | Avoid                                         |
| --------------------------- | ---------------------- | --------------------------------------------- |
| Local app backend           | Desktop service        | Bot backend                                   |
| Browser/game connection     | Game connection        | Dashboard connected when only UI is connected |
| Stop future scheduling      | Pause all automations  | Lock Bot                                      |
| Finish at safe boundary     | Stop after safe step   | Stop, without consequence                     |
| One run                     | Execution or operation | Job, run, action interchangeably              |
| User-readable record        | Activity               | Log when not technical                        |
| Detailed path               | Execution trace        | Raw log                                       |
| Technical evidence          | Diagnostics / Raw logs | Activity                                      |
| Cached state                | Last known · timestamp | Live                                          |
| Eligibility switch          | Enabled / Disabled     | Running / Off                                 |
| Commercial permission       | Entitlement            | License for every account state               |
| Slot-consuming installation | Licensed device        | Session                                       |

### Message style

- Lead with outcome.
- State scope and consequence.
- Use player/game nouns.
- Put internal IDs under technical disclosure.
- Prefer exact next action.
- Use neutral, non-blaming language.
- Avoid celebration for routine automation.
- Avoid “Oops,” “Uh-oh,” and vague failure copy in operations.
- Do not call a user error when the system can prevent it.

---

## Recommended screen set and blueprints

These are the screens worth prototyping after the research report. They cover the highest-risk
product concepts and the full brand ecosystem.

### Screen 1: Public home

**Purpose:** Establish value, trust, and a path to download.

```text
┌─────────────────────────────────────────────────────────────────────┐
│ Mark  Product  Automations  Pricing  Docs  Security   Sign in       │
├─────────────────────────────────────────────────────────────────────┤
│ CALM COMMAND OVER COMPLEX GAME OPERATIONS                            │
│ Supervise every castle, automate within your rules, and understand   │
│ every decision.                                                       │
│ [Download for Windows] [See how it works]                             │
│                                                                       │
│ ┌──────── Authentic product proof: overview + attention + next ────┐ │
│ └───────────────────────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────────────────┤
│ Observe clearly      Automate safely       Recover confidently       │
│ Live/stale state     Budgets and dry run   Trace and receipts         │
├─────────────────────────────────────────────────────────────────────┤
│ How it works: State → Policy → Plan → Confirmed outcome               │
├─────────────────────────────────────────────────────────────────────┤
│ Pricing summary  ·  Security  ·  Changelog  ·  Status                 │
└─────────────────────────────────────────────────────────────────────┘
```

### Screen 2: Account overview and devices

**Purpose:** Make entitlement, renewal, devices, and exceptions explicit.

```text
┌──────────── Account ────────────┬────────────────────────────────────┐
│ Overview                        │ Account overview                    │
│ Devices                         │ Plan: Command · Active              │
│ Plan & billing                  │ Renews Aug 15 · $X total            │
│ Security & sessions             │ 2 of 3 device slots used            │
│ Downloads                       │                                    │
│ Support                         │ Attention                           │
│ Data & privacy                  │ Desktop-PC is 3 versions behind     │
│                                 │ [Review update]                     │
│                                 │                                    │
│                                 │ Licensed devices                    │
│                                 │ This PC · current · 2m ago          │
│                                 │ Laptop · 31d ago       [Revoke]    │
└─────────────────────────────────┴────────────────────────────────────┘
```

### Screen 3: Empire Overview

**Purpose:** Answer safety, attention, current work, and next work in seconds.

```text
┌──── Navigation ────┬─────────────────────────────────────────────────┐
│ Overview            │ Connected · Live 8s ago    [Pause automations] │
│ Castles             ├─────────────────────────────────────────────────┤
│ Automations         │ Needs attention (2)                            │
│ Combat              │ Auto TCI blocked at Winter Keep  [Resolve]     │
│ Intelligence        │ Food reserve at risk in 34m       [Review]     │
│ Library             ├─────────────────────────────────────────────────┤
│                     │ Castles                                         │
│ System              │ GhostTown  Healthy · Recruiting · next 2m      │
│                     │ Winter Keep Blocked · no matching item          │
│                     │ Stonewatch  Waiting · slot free in 18m          │
│                     ├─────────────────────────────────────────────────┤
│                     │ Upcoming            Recent significant activity │
│                     │ Timeline/queue      Human-readable receipts     │
└─────────────────────┴─────────────────────────────────────────────────┘
```

### Screen 4: Automation control center

**Purpose:** Replace feature switches with visible lifecycle and reason.

```text
┌──── Navigation ────┬─────────────────────────────────────────────────┐
│ Automations         │ Automations      9 enabled · 2 need attention  │
│  Control center     │ [Pause all] [Create automation]                │
│  Schedules          ├─────────────────────────────────────────────────┤
│  Allocation         │ Filter: All  Attention  Running  Waiting       │
│  Templates          ├─────────────────────────────────────────────────┤
│                     │ Auto TCI                          Waiting        │
│                     │ All castles · next check 18m                    │
│                     │ 1 blocked: required item unavailable            │
│                     │ Last: Workshop upgraded · 3m                    │
│                     │ [Pause] [Inspect issue] [Settings]              │
│                     ├─────────────────────────────────────────────────┤
│                     │ Auto Recruit                      Running        │
│                     │ 2 castles · step 2 of 4 · confirms in ~8s       │
│                     │ [Stop after safe step] [View execution]         │
└─────────────────────┴─────────────────────────────────────────────────┘
```

### Screen 5: Execution detail and recovery

**Purpose:** Explain a run without raw logs and recover without duplication.

```text
┌──── Execution 8F2A ──────────────────────────────────────────────────┐
│ Auto TCI v8 · Winter Keep · Failed                                  │
│ Based on live state revision 18294 · Started 10:41:22               │
├───────────────────────────────────────────┬──────────────────────────┤
│ Timeline                                  │ Summary                  │
│ ✓ Reserve condition passed               │ No currency was spent    │
│ ✓ Construction slot confirmed free       │ Other castles unaffected │
│ ✓ Item equipped                           │                          │
│ ✕ Upgrade rejected by game               │ Recommended recovery     │
│   Item is no longer eligible              │ Refresh state and retry  │
│                                           │ failed safe step only    │
│ [Activity] [Technical details]            │ [Preview recovery]       │
└───────────────────────────────────────────┴──────────────────────────┘
```

### Screen 6: Activity and secondary diagnostics dock

**Purpose:** Keep human events primary and technical logs available.

```text
┌──────── Main workspace ──────────────────────────────────────────────┐
│                                                                     │
│                              ┌──── Activity / Execution / Logs ────┐ │
│                              │ Auto TCI · Winter Keep · 8F2A       │ │
│                              │ 10:42 Upgrade rejected              │ │
│                              │ 10:41 Item equipped                 │ │
│                              │ [Show surrounding context]          │ │
│                              │                                     │ │
│                              │ Technical details ▸                 │ │
│                              │ Search · Severity · Channel · Time  │ │
│                              │ 12 new events [Jump to latest]      │ │
│                              └─────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
```

### Screen 7: Automation editor and dry run

**Purpose:** Validate When/If/Then/limits and show exact effect.

```text
┌──── Auto TCI editor ─────────────────────────────────────────────────┐
│ Draft v9 · Live v8                 [Save draft] [Review & deploy]    │
├───────────────────────────────┬───────────────────────────────────────┤
│ 1 When                        │ Live preview                          │
│ 2 If                          │ 3 castles in scope                    │
│ 3 Then                        │ 1 expected action in next hour        │
│ 4 Scope & resources           │ 1 blocked dependency                  │
│ 5 Limits & stop behavior      │ Up to 12,500 silver · premium blocked │
│ 6 Failure behavior            │ Input live 6s ago                      │
│                               │ [Run dry test]                         │
└───────────────────────────────┴───────────────────────────────────────┘
```

### Screen 8: Browser device authorization

**Purpose:** Make desktop sign-in identity, device, scope, and recovery explicit.

```text
┌──────────────────────────────────────────────────────────────────────┐
│ Mark  Authorize CitadelOps Desktop                                   │
├──────────────────────────────────────────────────────────────────────┤
│ Sign in on Nebula-PC                                                 │
│ Windows 11 · CitadelOps 2.0.0                                       │
│                                                                      │
│ The desktop app will access your Citadel account and entitlements.   │
│ Your game password is not sent to this website.                      │
│                                                                      │
│ Continue as account@example.com                                     │
│ [Authorize this device] [Use another account] [Cancel]               │
│                                                                      │
│ Having trouble? [Use a one-time code]                                │
└──────────────────────────────────────────────────────────────────────┘
```

### Screen 9: Past-due/grace licensing state

**Purpose:** Exercise a high-consequence state instead of designing only the happy path.

```text
┌──────────── Account ────────────┬────────────────────────────────────┐
│ Overview                        │ Payment needs attention             │
│ Devices                         │ Grace ends Jul 21 at 11:59 PM EDT   │
│ Plan & billing                  │                                    │
│ Security & sessions             │ Your saved data and history remain │
│ Downloads                       │ available. New automation launches  │
│ Support                         │ will pause when grace ends.         │
│ Data & privacy                  │                                    │
│                                 │ Amount due: $X total                │
│                                 │ [Update payment] [View invoice]     │
│                                 │                                    │
│                                 │ This is a billing issue—not a game │
│                                 │ or desktop connection failure.     │
└─────────────────────────────────┴────────────────────────────────────┘
```

### Screen 10: Entitlement service unavailable

**Purpose:** Prove that an outage is not misrepresented as an invalid license.

```text
┌──────────────────────────────────────────────────────────────────────┐
│ Entitlement check unavailable                                       │
│ Last validated Jul 14 at 9:42 PM · offline grace: 2d 11h remaining  │
│                                                                      │
│ Your entitlement has not been marked invalid.                       │
│ Existing safe operations remain visible; high-value new writes are  │
│ paused until validation returns. Automatic retry in 2m.              │
│                                                                      │
│ [Retry now] [View service status] [Work offline]                     │
└──────────────────────────────────────────────────────────────────────┘
```

### Cohesion requirements for all concept screens

- Same provisional Citadel Monogram symbol.
- Same Command Violet brand/action and semantic palette.
- Same typography, 8/4 grid, radius, and icon grammar.
- Public site uses expressive scale; account portal quiet scale; desktop productive scale.
- Green appears only for confirmed healthy/success.
- No raw logs on home.
- Every status has text and icon.
- Every live value has freshness where it matters.
- Every material action exposes scope and consequence.
- Glass is limited to shell/transient layers.
- Both Light and Dark mappings are documented, even if the first concept board uses Dark app + Light
  public/account examples.

---

## Assumptions and open product decisions

These questions can materially change the final interface. The concept screens use conservative
assumptions but must not silently hard-code them into product policy.

### Users and roles

- Is each account strictly single-user, or can an alliance/team share operations?
- If team roles exist, who can view, edit policy, approve spending, pause, inspect logs, manage
  billing, and revoke devices?
- Is there one game account per Citadel account or several?
- Can one installation switch between accounts, profiles, or servers?
- Which actions require reauthentication or a second approver?

**Concept assumption:** single primary operator, with no collaborative roles shown.

### Supported environment

- Which Windows versions, browsers, screen sizes, DPI scales, and input modes are supported?
- Are macOS/Linux planned or only shown as unavailable downloads?
- What happens when the upstream game changes protocol or blocks compatibility?
- Does the local service run when the UI is closed?

**Concept assumption:** Windows-first desktop with system-browser sign-in and background-capable
local service.

### Cloud and data boundaries

- Which data remains only local?
- Does normalized game state sync to an account service?
- Are definitions, presets, activity, logs, or support bundles cloud-backed?
- What is required telemetry versus optional analytics/crash reporting?
- What are retention, export, deletion, backup, and encryption policies?

**Concept assumption:** game credentials remain local; account cloud holds identity, entitlement,
device, billing, and support metadata; diagnostic upload is optional and redacted.

### Licensing and commerce

- Trial, plan, billing interval, currency, tax, refund, and cancellation model.
- Number and definition of device slots.
- Offline-license grace and past-due grace.
- Access policy for canceled, expired, revoked, server-unavailable, and refunded states.
- Whether free/read-only mode exists.
- Regional checkout and consumer-law requirements.

**Concept assumption:** subscription entitlement with named device slots, transparent period-end
cancellation, short offline/past-due grace, and retained read/export/support access.

### Automation and AI policy

- Which game writes are reversible, high-value, purchase, or launch effects?
- Which can be preapproved within budgets?
- How are commander/slot/currency claims prioritized?
- What is the exact master-pause and safe-stop contract?
- Which AI permission levels can exist, and which actions are always prohibited?
- How is unknown upstream command outcome reconciled?

**Concept assumption:** AI recommends/drafts by default; material game writes require typed intent,
fresh-state preview, policy approval, and receipt.

### Brand and rights

- Is the CitadelOps name retained, modified, or replaced after clearance?
- Which game names, screenshots, icons, and art may be used in public marketing?
- Which symbol, wordmark, type family, and color territory pass competitor/user tests?
- Which jurisdictions and classes require protection?

**Concept assumption:** “CitadelOps” is a working name and every identity asset is provisional.

### Support and operations

- Support channels, hours, service levels, and escalation.
- Public status ownership and incident update cadence.
- Vulnerability disclosure and security contact.
- Update channel, signing, rollback, and emergency release policy.
- Maximum local log/receipt retention and support-bundle size.

---

## Implementation roadmap

### Phase 0: Resolve product and brand gates

- Define launch audience, regions, business model, device policy, offline/grace policy, and AI
  authority boundaries.
- Conduct professional name/trademark clearance.
- Decide retain, modify, or rename before final identity investment.
- Interview novice and expert users; inventory high-risk workflows.
- Establish a canonical vocabulary and action-risk classification.

**Exit gate:** Naming risk accepted/resolved; core product policy decisions documented.

### Phase 1: State and IA foundation

- Define orthogonal lifecycle, attention, freshness, scope, actor, and availability models.
- Define connection-chain states.
- Define billing/subscription/entitlement/device/session states.
- Validate public/account/desktop IA with card sort and tree tests.
- Introduce durable app routes and deep-link model.

**Exit gate:** Users can locate urgent tasks and correctly interpret state/scope in low-fidelity
prototypes.

### Phase 2: Design-system foundations

- Implement semantic token layers and Light/Dark/System mappings.
- Separate brand/action from success.
- Fix typography, letter spacing, radius, focus, target size, dialog, select, navigation, and toggle
  primitives.
- Establish density, forced-colors, reduced-motion, and reduced-transparency behavior.
- Create status, alert, empty/loading/stale, activity, and automation-row primitives.

**Exit gate:** Component matrix passes accessibility across themes, density, and core states.

### Phase 3: Shell, Overview, and connection model

- New grouped routed shell.
- Quiet health summary and expandable connection detail.
- Explicit Connect/Disconnect and Pause/Resume semantics.
- Empire Overview and attention queue.
- Separate viewed castle from game focus or disclose the side effect.

**Exit gate:** Users identify system safety, freshness, and highest-risk exception quickly and
accurately.

### Phase 4: Automation control tower

- Expose server status detail, next check, last run, operation, error, and metrics.
- Standardize all automation lifecycle/persistence/telemetry behavior.
- Build filters, allocation view, schedules, and visible dependencies.
- Introduce global pause and safe-stop semantics.
- Remove privileged one-off automation controls from the global header.

**Exit gate:** Users distinguish enabled, waiting, running, blocked, failed, and paused without
opening settings or logs.

### Phase 5: Activity, traces, and diagnostics

- Human-readable activity model.
- Operation timeline and recovery choices.
- Correlation IDs/deep links.
- Secondary dock and dedicated diagnostics.
- Redacted support bundle.
- Explicit retention and pruning.

**Exit gate:** Users diagnose and recover representative failures without raw logs; support can
obtain consented evidence.

### Phase 6: Automation authoring and AI assistance

- Guided editor and advanced graph progressive disclosure.
- Draft/live/version model.
- Dry-run, test step, validation, and resource preview.
- Safe redrive/recovery.
- Contextual AI proposals, permission ladder, and intent integration.

**Exit gate:** No AI or automation path bypasses typed intents, resource policy, approval, or
receipts.

### Phase 7: Public website and account portal

- Public product, safety, pricing, download, docs, security, support, status.
- Passkey-ready auth and recovery.
- Entitlement, devices, billing/invoices, security sessions, privacy, tickets.
- Desktop browser handoff and entitlement refresh.
- Download signing/provenance and update UX.

**Exit gate:** Users correctly understand price, renewal, device slots, cancellation, security, and
desktop handoff.

### Phase 8: Identity completion and refinement

- Test three symbol territories after clearance.
- Build optical icon family and wordmark.
- Extend brand expression to product art and public site.
- Complete localization, pseudolocalization, responsive and accessibility testing.
- Optimize startup, state updates, images, logs, and long-session memory.

**Exit gate:** Brand is legally and perceptually differentiated; performance/accessibility budgets
pass.

---

## Validation program

### Research methods

- Contextual interviews and workflow observation.
- Task inventory by frequency, consequence, and difficulty.
- Card sorting and tree testing.
- First-click testing for urgent actions.
- Scenario-based think-aloud usability sessions.
- Alarm flood and flapping simulation.
- Degraded/offline connectivity exercises.
- Execution-trace comprehension testing.
- Destructive-action and recovery walkthroughs.
- Keyboard/screen-reader and accessibility audit.
- Brand territory and small-icon testing.
- Performance profiling under live updates and log load.

### Representative tasks

1. Determine whether the system and automation are safe to leave running.
2. Find the highest-risk castle issue.
3. Explain why one enabled automation is not acting.
4. Create and dry-run a conservative automation.
5. Identify scope, budget, and stop behavior before deploy.
6. Recover a failed execution without repeating completed work.
7. Find and share diagnostics for one incident.
8. Distinguish cached from live data during an outage.
9. Pause all automation and explain what happens to active work.
10. Revoke an old computer and free a device slot.
11. Find the next renewal amount/date.
12. Stop renewal and explain the access end state.
13. Determine whether an incident belongs to Citadel or the game provider.
14. Review an AI recommendation and correctly identify uncertainty and cost.

### Measures

- Task completion and first-click accuracy.
- Time to establish current state.
- Correct prioritization of the highest-risk item.
- Time to locate cause and recovery.
- Correct lifecycle/freshness interpretation.
- Correct castle/account/resource scope comprehension.
- Automation creation error rate.
- Accidental duplicate command rate.
- Alert acknowledgment versus real resolution.
- Recovery success without raw logs.
- Diagnostic completeness and redaction.
- Device-revocation success.
- Renewal/cancellation comprehension.
- Accessibility issue severity.
- INP/frame drops under sustained events.
- Self-reported confidence and trust calibration.
- NASA-TLX workload dimensions: mental, physical, temporal, performance, effort, frustration; see
  [NASA TLX](https://www.nasa.gov/human-systems-integration-division/nasa-task-load-index-tlx/).

### Research cohorts

- Newer players with no automation-tool experience.
- Experienced players new to CitadelOps.
- Current expert CitadelOps operators.
- Users who frequently diagnose technical failures.
- Keyboard-only and assistive-technology users.
- Users with color-vision differences and low vision.
- Users in target languages/regions.

Do not average away expert and novice failures. The design may need density and disclosure
differences, not one compromised mode.

---

## Anti-pattern checklist

Do not ship:

- A wall of telemetry or logs on Overview.
- Alerting on every error or expected event.
- Red/green as the only state cue.
- Brand action color reused as success.
- Constant glow, pulse, or animation for healthy/running state.
- Vanity charts with no decision.
- Badge counts containing routine history.
- Critical messages that auto-dismiss.
- Acknowledgment presented as resolution.
- One generic Offline state for every connection.
- Cached data presented as current.
- Disabled controls with no reason.
- Generic “Something went wrong.”
- Blind retry of completed purchase/upgrade steps.
- Hidden castle/account/resource scope.
- One switch representing enabled, running, healthy, and complete.
- Silent AI execution.
- A graph-first editor for simple recipes.
- Context-free onboarding tours.
- Modals and confirmations for routine low-risk actions.
- Marketing navigation or upsells in operational flows.
- Conflation of devices, sessions, subscription, and entitlement.
- Hidden total price or renewal terms.
- A cancellation retention maze.
- A download flow with no install/sign-in handoff.
- Blocking every launch for routine updates.
- A status page behind authentication.
- Support bundles that can leak secrets.
- Glass behind dense text or logs.
- A global 18-pixel radius overriding circles, pills, and compact controls.
- Hover-only critical status.
- Tiny muted operational text.
- Full dashboard rerenders on every live frame.
- Unbounded logs, activity, or operation receipts.

---

## Decision checklist before visual production

### Product

- [ ] Who is the launch user and what are their top five jobs?
- [ ] Which actions can spend premium/scarce resources?
- [ ] What is the master pause and safe-stop policy?
- [ ] Which data is local, account-cloud, optional telemetry, or support-only?
- [ ] What happens during game, local service, account service, and license outages?
- [ ] What AI permission levels are allowed?

### Brand

- [ ] Has the CitadelOps name been professionally cleared or replaced?
- [ ] Are domains, social handles, app listings, and design marks assessed?
- [ ] Have at least three identity territories been tested?
- [ ] Does the icon pass 16-pixel, monochrome, blur, mask, and competitor tests?

### UX

- [ ] Can users distinguish enabled, waiting, running, blocked, failed, and paused?
- [ ] Is every material action's scope explicit?
- [ ] Can common failures be recovered without raw logs?
- [ ] Are current, last-known, unknown, and stale distinct?
- [ ] Does each screen have one obvious state summary and primary task?

### Visual system

- [ ] Are brand/action and success different?
- [ ] Are all status colors redundant with icon and text?
- [ ] Are light, dark, forced-color, and reduced-transparency mappings defined?
- [ ] Does typography survive user spacing and localization expansion?
- [ ] Are radius, elevation, density, and motion scales purposeful?

### Accessibility gate

- [ ] Can core tasks be completed by keyboard?
- [ ] Do focus and dialogs behave correctly?
- [ ] Does 200% text/400% zoom work?
- [ ] Are live updates restrained and meaningful?
- [ ] Do targets, contrast, names, and errors meet the component contract?

### Performance gate

- [ ] Are heavy routes and catalogs loaded on demand?
- [ ] Are state subscriptions scoped?
- [ ] Are lists/logs virtualized and retention bounded?
- [ ] Is hidden polling suspended?
- [ ] Does the app stay smooth with WebSocket traffic and logs open?
- [ ] Is long-session memory stable?

### Account and commerce

- [ ] Are subscription, billing, entitlement, device, and session distinct?
- [ ] Are price, renewal, grace, cancellation, and end-of-access consequences explicit?
- [ ] Does license-service outage differ from invalid entitlement?
- [ ] Are auth, recovery, revocation, and diagnostic upload accessible and secure?

---

## Source library

This is a curated research library rather than an exhaustive bibliography. Links are grouped by the
decision they support.

### Human factors, automation, and control rooms

- [NASA Crew Interfaces, Volume 2](https://www.nasa.gov/reference/10-0-crew-interfaces-vol-2/)
- [NASA automation surprise and mode awareness research](https://ntrs.nasa.gov/citations/20040112190)
- [NASA adaptive automation review](https://ntrs.nasa.gov/citations/20060053373)
- [NASA mode awareness](https://shemesh.larc.nasa.gov/fm/fm-collins-mode.html)
- [NIST situation-awareness methodology](https://www.nist.gov/publications/evaluation-human-robot-interface-development-situational-awareness-methodology)
- [NRC NUREG-0700 Revision 4](https://www.nrc.gov/reading-rm/doc-collections/nuregs/staff/sr0700/index)
- [ISA-18 alarm-management standards](https://www.isa.org/standards-and-publications/isa-standards/isa-18-series-of-standards)
- [Google SRE monitoring](https://sre.google/workbook/monitoring/)
- [Grafana dashboard best practices](https://grafana.com/docs/grafana/latest/visualizations/dashboards/build-dashboards/best-practices/)

### General UX and information architecture

- [NN/g Ten Usability Heuristics](https://www.nngroup.com/articles/ten-usability-heuristics/)
- [NN/g Progressive Disclosure](https://www.nngroup.com/articles/progressive-disclosure/)
- [NN/g IA Study Guide](https://www.nngroup.com/articles/ia-study-guide/)
- [NN/g Menu Design](https://www.nngroup.com/articles/menu-design/)
- [NN/g Flat versus Deep Hierarchies](https://www.nngroup.com/articles/flat-vs-deep-hierarchy/)
- [NN/g Onboarding Tutorials](https://www.nngroup.com/articles/onboarding-tutorials/)
- [ISO 9241-210 human-centred design](https://www.iso.org/standard/77520.html)
- [GOV.UK service navigation](https://design-system.service.gov.uk/patterns/navigate-a-service/)

### Automation and workflow products

- [AWS Step Functions Workflow Studio](https://docs.aws.amazon.com/step-functions/latest/dg/workflow-studio.html)
- [AWS execution details](https://docs.aws.amazon.com/step-functions/latest/dg/concepts-view-execution-details.html)
- [AWS execution redrive](https://docs.aws.amazon.com/step-functions/latest/dg/redrive-executions.html)
- [Home Assistant automation editor](https://www.home-assistant.io/docs/automation/editor/)
- [Home Assistant traces and troubleshooting](https://www.home-assistant.io/docs/automation/troubleshooting/)
- [n8n executions](https://docs.n8n.io/workflows/executions/all-executions/)
- [Node-RED editor](https://nodered.org/docs/user-guide/editor/)

### Logs, activity, and observability

- [OpenTelemetry Logs Data Model](https://opentelemetry.io/docs/specs/otel/logs/data-model/)
- [Elastic Explore Logs](https://www.elastic.co/guide/en/observability/current/explore-logs.html)
- [Grafana Logs in Explore](https://grafana.com/docs/grafana-cloud/visualizations/explore/logs-integration/)
- [Grafana Explore](https://grafana.com/docs/grafana/latest/visualizations/explore/get-started-with-explore/)
- [Grafana correlations](https://grafana.com/docs/grafana/latest/administration/correlations/)
- [Datadog log facets](https://docs.datadoghq.com/logs/explorer/facets/)
- [Node-RED debug sidebar](https://nodered.org/docs/user-guide/editor/sidebar/debug)

### Color, typography, layout, motion, and visualization

- [CSS Color 4](https://www.w3.org/TR/css-color-4/)
- [DTCG 2025.10](https://www.designtokens.org/)
- [DTCG Color Module](https://www.w3.org/community/reports/design-tokens/CG-FINAL-color-20251028/)
- [USWDS color overview](https://designsystem.digital.gov/design-tokens/color/overview/)
- [USWDS state tokens](https://designsystem.digital.gov/design-tokens/color/state-tokens/)
- [Carbon color usage](https://carbondesignsystem.com/elements/color/usage/)
- [Carbon typography](https://carbondesignsystem.com/elements/typography/type-sets/)
- [Carbon productive/expressive strategies](https://carbondesignsystem.com/elements/typography/style-strategies/)
- [Carbon 2x grid](https://carbondesignsystem.com/elements/2x-grid/overview/)
- [Carbon motion](https://carbondesignsystem.com/elements/motion/overview/)
- [Atlassian motion](https://atlassian.design/foundations/motion)
- [Apple Materials](https://developer.apple.com/design/human-interface-guidelines/materials)
- [USWDS data visualizations](https://designsystem.digital.gov/components/data-visualizations/)
- [ColorBrewer schemes](https://colorbrewer2.org/learnmore/schemes_full.html)

### Accessibility and game-adjacent UX

- [WCAG 2.2](https://www.w3.org/TR/WCAG22/)
- [WCAG Understanding documents](https://www.w3.org/WAI/WCAG22/Understanding/)
- [Xbox Accessibility Guidelines](https://learn.microsoft.com/en-us/xbox/accessibility/guidelines)
- [XAG Text Display](https://learn.microsoft.com/gaming/accessibility/xbox-accessibility-guidelines/101)
- [XAG Redundant Cues](https://learn.microsoft.com/en-us/xbox/accessibility/xbox-accessibility-guidelines/103)
- [XAG Input](https://learn.microsoft.com/en-us/xbox/accessibility/xbox-accessibility-guidelines/107)
- [XAG UI Navigation](https://learn.microsoft.com/en-us/xbox/accessibility/xbox-accessibility-guidelines/112)
- [WAI-ARIA Authoring Practices](https://www.w3.org/WAI/ARIA/apg/)

### AI interaction and risk

- [Microsoft Guidelines for Human-AI Interaction](https://www.microsoft.com/en-us/research/publication/guidelines-for-human-ai-interaction/)
- [Google PAIR Explainability and Trust](https://pair.withgoogle.com/guidebook-v2/chapter/explainability-trust/)
- [Google PAIR Feedback and Control](https://pair.withgoogle.com/guidebook-v2/chapter/feedback-controls/)
- [NIST AI Risk Management Framework](https://airc.nist.gov/airmf-resources/airmf/)

### Authentication, licensing, privacy, and commerce

- [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html)
- [WebAuthn Level 3](https://www.w3.org/TR/webauthn-3/)
- [OAuth for Native Apps](https://datatracker.ietf.org/doc/html/rfc8252)
- [OAuth Device Authorization](https://datatracker.ietf.org/doc/html/rfc8628)
- [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [OWASP Forgot Password Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html)
- [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)
- [Stripe Subscription Lifecycle](https://docs.stripe.com/billing/subscriptions/overview?locale=en-GB)
- [Stripe Entitlements](https://docs.stripe.com/billing/entitlements?dashboard-or-api=api)
- [Stripe Customer Portal](https://docs.stripe.com/customer-management)
- [FTC Dark Patterns Report](https://www.ftc.gov/reports/bringing-dark-patterns-light)
- [ICO layered privacy information](https://ico.org.uk/for-organisations/uk-gdpr-guidance-and-resources/individual-rights/the-right-to-be-informed/what-methods-can-we-use-to-provide-privacy-information/?q=retention)

### Brand, trademark, and icon design

- [USPTO Strong Trademarks](https://www.uspto.gov/trademarks/basics/strong-trademarks)
- [USPTO Federal Trademark Searching](https://www.uspto.gov/trademarks/search/federal-trademark-searching)
- [USPTO Likelihood of Confusion](https://www.uspto.gov/trademarks/search/likelihood-confusion)
- [USPTO Comprehensive Clearance Search](https://www.uspto.gov/trademarks/search/comprehensive-clearance-search-similar-trademarks)
- [WIPO Global Brand Database](https://www.wipo.int/en/web/global-brand-database)
- [Apple App Icons](https://developer.apple.com/design/human-interface-guidelines/app-icons)
- [Windows App Icon Design](https://learn.microsoft.com/en-us/windows/apps/design/iconography/app-icon-design)
- [Web App Manifest](https://www.w3.org/TR/appmanifest/)
- [Aaker, Dimensions of Brand Personality](https://www.gsb.stanford.edu/faculty-research/publications/dimensions-brand-personality)

### Performance sources

- [Core Web Vitals](https://web.dev/articles/vitals?hl=en)
- [Core Web Vitals thresholds](https://web.dev/articles/defining-core-web-vitals-thresholds?hl=en)
- [RAIL](https://web.dev/articles/rail)
- [Rendering Performance](https://web.dev/articles/rendering-performance)
- [Animation Performance](https://web.dev/articles/animations-guide)
- [DOM Size and Interactivity](https://web.dev/articles/dom-size-and-interactivity)
- [Virtualizing Long Lists](https://web.dev/articles/virtualize-long-lists-react-window)
- [CSS content-visibility](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/content-visibility)

### Localization

- [Unicode CLDR Plural Rules](https://cldr.unicode.org/index/cldr-spec/plural-rules)
- [Microsoft Pseudolocalization](https://learn.microsoft.com/en-us/globalization/methodology/pseudolocalization)
- [W3C Strings and Bidirectional Text](https://www.w3.org/international/articles/strings-and-bidi/)

---

## Final recommendation

Begin implementation with **state semantics, routing, and component foundations**, not a visual
reskin. Then build the Overview, Automation control tower, and activity/trace hierarchy that reveal
the product's existing deterministic engine. In parallel, resolve the naming risk before final
identity work and define the account/licensing policies that the public ecosystem must explain.

The proposed visual direction—neutral-first surfaces, restrained command violet, green reserved for
confirmed success, compact productive typography, limited glass, and a provisional Citadel
Monogram—provides a coherent basis for concept screens. It should remain a hypothesis until it
passes name clearance, accessibility, small-icon, competitor, novice/expert, and live-performance
testing.

The central rule for every surface is:

> **State first. Action second. Execution trace third. Raw diagnostics last.**
