# Automation rejection safety

This is a runtime policy, independent of error-message catalogs. Knowing what an
error means is not permission to recover automatically.

| Response | Automatic behavior |
| --- | --- |
| Success (0) | Existing success handling |
| ADI 95 | Existing target-cooldown recovery |
| ERE 227 / EQE 227 | Existing failed-enchantment recovery, with reserve checks before retries |
| BUP 87 | Existing recruitment rejection handling; no safety lane lock |
| AHR 273 | Duplicate/multiple alliance-help rejection; failure remains visible without a new safety lane lock |
| Any non-whitelisted nonzero rejection, including MSD and CRA 256 | Originating lane locked for 30 minutes from the rejection; no early manual clear |

These are the only five approved opcode/code pairs. Approval bypasses the safety
lock, not rejection handling: errors remain failures unless their existing intent
policy explicitly permits recovery. No other pair becomes safe because it shares
a numeric code or has a known catalog description. Further exceptions require
separate stress-test evidence and approval; no live stress testing is enabled here.

AHR 273 was explicitly approved on 2026-09-12 after matching the official client
constant `NO_MULTIPLE_ALLIANCEHELP` to a captured recruitment request. This narrow
tenant hotfix is queued for the 2.4.0 desktop release. It adds no retry and does not
declare the rejected operation successful. The follow-up lifecycle hotfix makes
all non-whitelisted locks expire after 30 minutes. Whitelisted saved incidents
are ignored by every runtime guard and automatically released during startup.

The coordinator supplies the exact policy ID separately from its shared actor.
Main operations, follow-ups, dependencies and failure fallbacks retain that lane
identity. The intent engine checks locks before planning/execution and at final
dispatch, and records rejections before retry, stale-plan recovery or compensation.
Earlier confirmed progress remains in the receipt; unresolved reservations remain
available for review/reconciliation. Independent lanes are not disabled.

Lane identity is mandatory for automated submissions: a missing lane is rejected
before execution, never inferred from the parent actor/feature. For example,
`autoKhan:cooldown` cannot lock `autoKhan`, `autoKhan:rage` or `autoKhan:defense`;
`autoStormShop` cannot lock the attack or build lanes. Shared feature enablement
and schedules are not modified by a rejection. A sibling may still wait for its
ordinary prerequisites, but it does not inherit another lane's safety lock.

Automated wire steps without an explicit response wait now wait for their command
opcode. Commands with aliased success responses also watch for rejections under
the original command name. Both transports correlate unique alias rejections;
ambiguous replies are never assigned to an arbitrary operation.

Locks record lane, opcode, code, triggering operation, intent, observation time,
reason, expiry and review details in the account's automation state. Writes are
forced through profile persistence even if the operation was canceled. Reloads
retain the original observation time plus 30 minutes; configuration toggles and
reconnects do not restart the timer. Adoption retains the latest active effective
expiry. Startup durably refreshes legacy indefinite locks, audits release of
expired/whitelisted incidents, and retains their original operation evidence.
Timestamp-free malformed records get one persisted 30-minute timer at refresh;
normal records never receive a fresh timer. An unreadable profile or failed
refresh persistence prevents startup rather than dropping safety evidence.

The Automation page exposes active locks and their expiry. `automation.safety.clear` requires a
manual actor, exact triggering operation ID and a nonempty review (max 1000
characters). A stale screen cannot clear a newer incident. A failed durable clear
restores the lock. Manual review cannot shorten an active 30-minute cooldown.
Clearing an incident does not add its error pair to the policy.
The triggering failure creates one alert; subsequent blocked operations stay quiet.

Deployment is separate from implementation. This branch does not alter running
tenants or the future-cell template. Every upgraded runtime uses this policy;
cross-cell moves must retain the durable player profile. Dashboard checkpoints
alone are not evidence of profile restoration. Add new recovery exceptions only
after reviewing exact opcode/code captures and proving their bounded recovery.

## Validation

Focused coverage includes exact pair matching, unknown and known-but-unapproved
errors, prior progress, chain/retry/compensation suppression, isolated lanes,
aliased replies, canceled-operation persistence, restart and profile adoption,
stale review, storage failure, mandatory MSD expiry, and notification deduplication.

The date-sensitive State persistence fixture now stays inside the production
retention window without changing retention behavior. The baseline has 43 strict
client TypeScript diagnostics, reproduced against unchanged source; this change
adds none. The normal client production build and focused server suites pass.
Deployment requires separate immutable candidate and live rollout receipts.
