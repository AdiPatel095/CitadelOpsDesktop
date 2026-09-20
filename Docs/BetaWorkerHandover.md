# Beta worker handover implementation status

## Settings-only switching (current model)

Stable and beta remain separate VMs with separate persistent profile disks.
New channel switches use `settings-only`: stop/fence the source, reserve the
destination's existing local profile (create one only on its first visit),
commit the placement, and start the destination parked until it acknowledges
the latest canonical settings revision and digest. Settings are never seeded
from the destination's older local revision. The account, license, canonical
database and cloud history remain shared. Local logs, operation databases,
history and game-state files do NOT move or merge between the two VMs.

`Accounts/local-profile-bindings.json` retains the per-cell directory and its
durable ProfileID across returns and restarts. The source fence and exact next
epoch remain mandatory. A switch waits for active rejection safety locks to
expire rather than allowing a different local profile to bypass them. Source
and target builds advertise `settingsSwitchSchema: 1`; no automatic archive
fallback is used for new requests. Readiness requires fresh game state, exact
settings and published canonical metrics/checkpoints; repeated polling does
not reset an already-current worker publication state.

The archive implementation below is retained ONLY to safely finish operations
already accepted under the old protocol. Do not reset those journals or
overwrite profiles. Amos's already-restored beta profile remains on beta, and
his stable profile remains on stable. Rollback after activating a local binding
must retain settings-only binding and fencing support.

This document describes implementation, not a live deployment receipt.

## Legacy full-profile transfer

This implements the worker half of the gated account-switch feature; it is
**not a deployment receipt**. Handover routes default off and require explicit
`CITADEL_ENABLE_HANDOVER_TRANSPORT=true`. No runtime channel
is changed by visiting the beta portal, loading this build, or calling the
existing reconnect action.

## Implemented here

- `Runtime.WriteProfileArchive` captures a pre-existing, stopped profile under
  the application's exclusive profile lease. It includes every directory and
  regular file except the process-local `Runtime/Profile.lock` file. It preserves
  the durable ProfileID and binds the archive to an exact operation, account,
  runtime, tenant, source cell/epoch and canonical configuration revision/digest.
- `Runtime.RestoreProfileArchive` validates into a new private staging directory.
  It verifies the whole-archive and per-file SHA-256 values and profile identity;
  it rejects links, unsafe/duplicate paths, unexpected entries, corrupt/truncated
  content, unsupported schemas and mismatched operations. Failed stages are
  removed; existing destinations are never overwritten. Files use mode 0600,
  directories 0700, with file and directory synchronization before success.
- Limits are 100,000 entries, 16 GiB of file content, a 32 MiB manifest and bounded
  archive overhead. Oversized profiles fail closed. Capture currently requires
  Unix link-count metadata, matching hosted Linux workers; Windows is not an
  enabled profile-capture target.
- `Accounts.PrepareSourceHandover` validates the current placement and applied
  canonical configuration, persists the source fence before stopping, revokes
  dashboard access, waits for shutdown/profile release, and verifies the durable
  profile identity before acknowledging the stop. Retries bind to the same
  operation and exact configuration. Only the gated internal transfer transport
  calls it over HTTP. Metadata preflight rejects known oversize or unsafe profiles
  before source fencing, with a 256 MiB shutdown-growth margin.
- Fences survive supervisor/process recreation. They block the retired runtime,
  aliases of its player profile, rebinds onto that profile, stale per-runtime
  control calls, and arbitrary higher-epoch reconciles. A verified, explicitly
  activated imported generation can supersede runtime ownership; retired
  directories remain permanently fenced. Corrupt/non-canonical journals fail
  startup rather than silently forgetting ownership.
- `Accounts.RestoreTargetProfile` reserves capacity and fences the target before
  accepting an archive. It restores into a newly-created private stage and
  verifies the exact receipt, durable ProfileID and every pinned canonical
  configuration section without opening/migrating the settings store. Local
  settings revision and unknown/local sections are preserved byte-for-byte at
  restore; they are not substituted for the backend's canonical revision.
- Every incoming operation gets a new `Accounts/transfer-<operation>` directory.
  Atomic rename plus file/directory synchronization precedes the durable
  `restored` receipt. A retry after an interrupted rename re-hashes the existing
  generation; it never copies retry bytes over it. Ordinary accounts cannot
  consume reserved capacity or open/alias/rebind onto these directories.
- `Accounts.ActivateTargetProfile` only publishes the durable profile pointer.
  Its trusted caller must first commit the backend target placement. Activation
  itself creates no App, grants, login or game connection. Reconcile must then
  name the exact target epoch and tenant, and configuration/login follow the
  existing parked-runtime acknowledgement gates. Missing durable ProfileID
  fails closed instead of creating a fresh profile after restart.
- Reverse transfers use a new operation and the latest stopped profile. The
  adoption journal retains every generation and retired directory. Old source
  stop requests, activation retries after departure, older epochs and arbitrary
  epoch increases cannot resurrect or stop a different generation. Imported
  profiles bypass player-directory rebind/merge entirely.
- A supervisor holds an exclusive process-root lease before reading ownership
  journals. Per-profile leases alone cannot prevent two supervisors selecting
  different generations. Clean shutdown releases it only after account and
  shared-store shutdown; failed shutdown retains ownership until retry/exit.
  Rollback must retain this lease/adoption support once transfers are used.
- Private transport schema v1 requires the explicit `EnableHandoverTransport`
  composition or its opt-in CLI environment flag. It registers authenticated
  POST preflight/export/download/restore/activate routes under
  `/orchestrator/v1/handovers/`. Every route requires a positive controller epoch
  and shares the full-request persisted controller fence. Capability is absent
  from ordinary status when disabled; disabled requests cannot mint an epoch.
- Source export captures one immutable archive after acknowledged stop and
  publishes the tar plus compact receipt using private files, fsync and atomic
  directory rename. Exact retries re-hash the saved file, never re-capture later
  source mutations. Downloads bind identity, receipt, whole-file SHA and length.
  Corrupt exports fail closed; they are not replaced automatically.
- Target restore accepts bounded multipart metadata/archive parts, validates the
  complete archive and pinned configuration, and returns only a stopped restore
  receipt. Activation is separate. Responses are no-store, errors contain no
  raw archive/configuration/credential content, and no public archive URL exists.
- Controller fencing protocol v1: authenticated mutations carrying canonical
  `X-Citadel-Control-Epoch` values are serialized under an interprocess file
  lock. The cell persists its high-water epoch with atomic rename/fsync before
  dispatch. Older and missing epochs are rejected after first activation;
  corrupt journals fail startup. Status advertises `controlFenceSchema` and
  `controlEpoch`. `POST /orchestrator/v1/control-fence` claims ownership without
  changing any runtime, for the backend's all-cell takeover barrier.
- Fenced reconciliation supports explicit `preserveRuntimes` IDs. It preserves
  their current assignments without creating absent Apps, changing grants or
  reviving stopped/fenced sources. Other siblings still reconcile normally.
- Enabled status reports available profile-volume bytes for controller admission.
  The controller requires two maximum-archive budgets free on both cells before
  accepting a move and permits only one pending fleet transfer. This is an
  admission check, not a filesystem quota or a guarantee against other writers.

Controller fencing is backward-compatible only **before** its first positive
epoch. It does not expire. Deploy support everywhere before enabling backend
`HOSTED_CONTROL_FENCING=true`; afterward rollback must retain fencing support.
Never delete/reset `Accounts/controller-fence.json` to make an old writer work.
The paired backend executor must be deployed and explicitly enabled before
account switching can be activated. Source archives remain private to workers.

## Still required before activation

1. Deploy and verify the backend journal/CAS and controller fence support across
   all replicas and cells; target capacity reservation and stale-grant rejection.
2. Review the paired backend executor and opt-in transport. Real offline HTTP
   integration covers complete profile preservation and executor placement CAS,
   activation, settings and credential installation while parked. It does not
   prove live game readiness.
3. Deploy exact immutable stable/beta build manifests with an explicitly reviewed
   profile-compatibility attestation; archive schema alone is insufficient.
4. Placement/grant transfer at a higher epoch, target startup and exact readiness
   checks (login/socket, generations, config, checkpoints, metrics, no failure).
5. End-to-end reverse execution using the latest beta state. Local synthetic
   stable/beta round trips cover generation adoption, retained history, restarts,
   stale requests and interrupted restores; live reverse readiness still needs
   real worker publications and a controlled live rehearsal.
6. Server-enforced frontend/channel compatibility and authenticated switch UI.
7. Reviewed compatible stable and beta worker builds through Cloud Build, source
   and artifact provenance, rollback receipts, dedicated beta capacity, and
   non-spending interruption/race/downgrade rehearsals before the first live move.

The canonical account, ownership, license, history and settings row/revision
must remain the same. A source fence is not proof of a completed transfer, and
an archive receipt is not proof that a target may start. Do not expose the source
stop method until the complete executor and recovery path are reviewed.

## Operational recovery boundaries

There is no unlock, expiry, overwrite or automatic deletion of old profiles.
`Accounts/profile-adoptions.json` is a bounded schema-v1 journal containing only
identities, receipts, generation pointers and retired paths. Reservations remain
closed after failure and require an exact retry; they do not time out into an
empty profile. A disk-full journal fails closed. Interrupted processes can leave
private `.profile-transfer-*` stages; do not delete them during an unresolved
handover. The executor/deployment gate must reserve disk space and define audited
orphan-stage cleanup before activation. The source/target archive schema alone
does not prove a future runtime data schema is downgrade-compatible.

Exports live at `Transfers/Exports/<operation>/` outside account/player roots.
Interrupted captures can leave private `.export-*` stages. Retain exports and
source profiles until an audited retention/recovery policy is implemented. The
backend relay streams directly between private worker connections; it does not
spool archives to SQL, public storage or frontend responses. Its client rejects
redirects, bounds responses/streams and requires the all-cell controller fence
to be already acknowledged before issuing transfer commands.
