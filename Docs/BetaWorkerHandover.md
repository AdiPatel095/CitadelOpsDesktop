# Beta worker handover implementation status

This is worker-side groundwork, **not an enabled account-switch feature**.
No new handover route is registered. No runtime channel is changed by visiting
the beta portal, loading this build, or calling the existing reconnect action.

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
- Limits are 100,000 entries, 2 GiB of file content, a 32 MiB manifest and bounded
  archive overhead. Oversized profiles fail closed. Capture currently requires
  Unix link-count metadata, matching hosted Linux workers; Windows is not an
  enabled profile-capture target.
- `Accounts.PrepareSourceHandover` validates the current placement and applied
  canonical configuration, persists the source fence before stopping, revokes
  dashboard access, waits for shutdown/profile release, and verifies the durable
  profile identity before acknowledging the stop. Retries bind to the same
  operation and exact configuration. It is not exposed over HTTP yet.
- Fences survive supervisor/process recreation. They block the retired runtime,
  aliases of its player profile, rebinds onto that profile, stale per-runtime
  control calls, and higher-epoch reconciles. Corrupt/non-canonical fence journals
  fail startup rather than silently forgetting source ownership.
- Controller fencing protocol v1: authenticated mutations carrying canonical
  `X-Citadel-Control-Epoch` values are serialized under an interprocess file
  lock. The cell persists its high-water epoch with atomic rename/fsync before
  dispatch. Older and missing epochs are rejected after first activation;
  corrupt journals fail startup. Status advertises `controlFenceSchema` and
  `controlEpoch`. `POST /orchestrator/v1/control-fence` claims ownership without
  changing any runtime, for the backend's all-cell takeover barrier.

Controller fencing is backward-compatible only **before** its first positive
epoch. It does not expire. Deploy support everywhere before enabling backend
`HOSTED_CONTROL_FENCING=true`; afterward rollback must retain fencing support.
Never delete/reset `Accounts/controller-fence.json` to make an old writer work.
The backend still needs the complete handover executor/target adoption before
account switching can be activated. These routes do not expose source archives.

## Still required before activation

1. Deploy and verify the backend journal/CAS and controller fence support across
   all replicas and cells; target capacity reservation and stale-grant rejection.
2. Authenticated, bounded archive delivery bound to the persisted stop receipt.
   Archive bytes must not enter public storage, logs or portal responses.
3. Target schema/identity/configuration verification, atomic adoption and the
   correct player-directory binding. Restore currently creates only a stage.
4. Placement/grant transfer at a higher epoch, target startup and exact readiness
   checks (login/socket, generations, config, checkpoints, metrics, no failure).
5. An explicit reverse protocol using the latest beta state. These source fences
   intentionally have no generic unlock or timeout expiry; restoring an old
   backup or merely increasing an epoch must not restart an obsolete source.
6. Server-enforced frontend/channel compatibility and authenticated switch UI.
7. Reviewed compatible stable and beta worker builds through Cloud Build, source
   and artifact provenance, rollback receipts, dedicated beta capacity, and
   non-spending interruption/race/downgrade rehearsals before the first live move.

The canonical account, ownership, license, history and settings row/revision
must remain the same. A source fence is not proof of a completed transfer, and
an archive receipt is not proof that a target may start. Do not expose the source
stop method until the complete executor and recovery path are reviewed.
