# Citadel Ops Discord support (CIT-99)

Standalone Mac LaunchAgent using Node **25.9.0 or newer**, pinned **discord.js 14.27.0**, and built-in `node:sqlite`. It does not use the game/client packages. The only Gateway intent is **Guilds**. No message event ingestion, transcripts, AI moderation, account/billing actions, channel deletion, expiry, or reopening.

Each modal request opens one private text channel. Independent requests from the same person are allowed. Only that requester, Support Team, and administrators have access by default. Discord administrators (including administrator bots) inherently bypass channel overwrites. Claim is first-writer-wins. Close requires a private, expiring confirmation and a fresh Support/Admin membership check; it retains the channel and requester view/history while denying posting and threads. Staff assignments are managed outside this service.

## Verification without Discord or credentials

From `tools/discord-support`:

```sh
npm ci --ignore-scripts
npm test
npm run check
npm audit --audit-level=low
npm run install-service -- --dry-run
```

Tests use fabricated identifiers, injected Discord adapters and private temporary directories. No live token, Discord call, launchd registration, or customer conversation is needed. The dry-run computes the package digest and displays the exact plist, absolute Node executable, source allowlist and destination without changing the host. Dependencies are isolated in this directory and its lockfile. No unrelated game tests are required.

## Install and operate (release owner only)

Implementation/QA does **not** authorize executing these live commands. After Sophie's source approval and Claire's merge, Miles installs from the approved revision under the separately authorized Mac hosting flow:

```sh
cd tools/discord-support
npm ci --ignore-scripts
npm run install-service -- --dry-run
npm run install-service
npm run status
npm run stop
# Unregister and remove only this LaunchAgent; retain source, config, logs and tickets:
npm run uninstall-service
```

Reinstalling identical pinned source while registered is a no-op. Install prepares an isolated source copy and runs `npm ci --ignore-scripts` before stopping the previous service. It promotes an atomic `service` symlink into a versioned release, writes the plist and registers it. Preparation failure leaves the old source/service intact; registration failure restores the previous symlink/plist and re-registers the previous service when it was loaded. Previous releases are retained; there is no automatic cleanup or state deletion. `source-pin.json` records the Git revision and SHA-256 digest of the exact service source allowlist; future worktree edits do not change the running copy. An installer kernel lock serializes updates. Only `com.citadelops.discord-support` is operated on.

Paths under the current user's home:

- `Library/Application Support/CitadelOpsDiscord/service`: stable source symlink.
- `Library/Application Support/CitadelOpsDiscord/releases/`: pinned source and dependencies, including recoverable previous versions.
- `Library/Application Support/CitadelOpsDiscord/config.json`: private configuration, created with the plan's fixed bot/guild/Support Team/App Support resource IDs. Unknown keys or differing IDs fail closed. Optional `tokenPath` must be absolute.
- `~/.config/citadel-ops-discord/bot-token`: default existing token file; never copied to source or plist. Only the running service reads it. Require a regular nonsymlink file owned by the current user with no group/other permissions (0600 recommended) and one link. It must contain a single nonempty token with no whitespace.
- `Library/Application Support/CitadelOpsDiscord/state/tickets.sqlite`: durable ticket/resource identity and pending intake needed to finish interrupted creation; pending subject/description are cleared when opening finishes. No interaction tokens or transcripts. Lifecycle audit contains only opaque ticket ID/action/time. State directories are 0700 and files 0600, with umask 0077 and SQLite FULL synchronous transactions.
- `Library/Application Support/CitadelOpsDiscord/state/owner.lock`: persistent inode held by macOS `/usr/bin/lockf -k` (BSD kernel advisory lock). Do not remove it. A pipe-tethered helper releases ownership on normal shutdown or owner death; the existence of a stale file/PID never blocks restart and a live holder is never evicted.
- `Library/Application Support/CitadelOpsDiscord/state/readiness.json`: private current PID/bot/guild/heartbeat, last successful validation, status and check counter.
- `Library/Application Support/CitadelOpsDiscord/logs/service.log`: safe event/error codes only, rotated at 1 MiB with two retained generations. No raw exceptions, request bodies, tokens, subjects, descriptions or customer IDs. launchd stdout/stderr go to `/dev/null` to prevent unbounded dependency output; diagnose using the safe log and receipt.
- `Library/LaunchAgents/com.citadelops.discord-support.plist`: absolute Node/source paths, RunAtLoad/KeepAlive, 30-second restart throttle, 35-second shutdown allowance, and no secrets/environment token. Requires the installed Node executable to remain at its pinned absolute path.

Status separates launchd registration, PID existence, and fresh Gateway readiness. A ready receipt must match the current launchd PID and configured identity, have a heartbeat less than 45 seconds old and validation less than 120 seconds old. Disconnect/reconnect clears readiness immediately. Reconciliation and all identity/privacy checks must finish before readiness returns. Validation runs at least every minute while connected; heartbeats run every 15 seconds. Network retries have bounded exponential backoff up to 30 seconds. Fatal privacy/identity/uncertainty faults stay unready and fail closed.

## Recovery and privacy

A request key binds the initiating button ID, guild and requester. A persistent private signing key authenticates modal IDs and close confirmations across restart. Every channel/message intent is written before its REST call. REST acceptance retries are disabled; recovery scans exact markers before doing anything else. A recorded ticket channel is never replaced, even if missing or mismatched. Discord text-channel topics and bot message footers contain opaque markers; embeds use allowed-mentions suppression, including hostile customer input. Public panel recovery scans recent bot messages; private initial-message recovery scans marker metadata through history without retaining customer messages.

Discord categories do not have topics. The exact-name `🎫 Private Tickets` category uses a bot-authored channel-creation audit reason as its recovery marker. Audit records last 45 days; recorded category IDs are reused and checked after that. If the local resource ID was never persisted and its audit marker is no longer discoverable, recovery fails closed. An arbitrary same-name category/channel is never adopted. Duplicate markers and altered overwrites require investigation, not deletion or exposure.

If a previous attempted creation has no discoverable marker, the service reports `*_UNCERTAIN` and will not blindly issue another POST. This covers failed/rejected as well as ambiguous calls conservatively. Restore connectivity and restart after the accepted marker becomes visible; if it remains absent, the release owner must investigate Discord and the private durable state before deciding a recovery action. Do not reset attempt flags, delete the store/lock, clone the state into a second running service, or replace a bound channel to bypass this protection.

For backup, stop/unregister the service, verify its process has exited, and make an encrypted/private copy of the entire config/state directory. Treat SQLite (including pending input and signing key) as sensitive. Restore the same intact database while stopped, preserve ownership/modes, then install/start the approved source and verify readiness. Do not reconstruct tickets from channel names or inspect real customer conversations during setup verification.

This Mac service is unavailable during sleep, logout, power-off or loss of connectivity. It does not change sleep settings, run caffeinate, open HTTP/tunnel listeners, or imply an always-online hosted service. Live acceptance remains a separate Miles-owned step: a labeled owner setup-test request → exactly one private channel → chat → claim → confirmed close/read-only requester, retained test state, launchd restart/reconciliation and no duplicate panel/ticket. Automated tests do not prove that live flow.

Official contracts consulted: [Node SQLite](https://nodejs.org/api/sqlite.html), [discord.js 14.27.0](https://discord.js.org/docs/packages/discord.js/14.27.0/Client:Class), [interaction responses](https://docs.discord.com/developers/interactions/receiving-and-responding), [permission hierarchy](https://docs.discord.com/developers/topics/permissions), [channel/message API](https://docs.discord.com/developers/resources/channel), [audit logs](https://docs.discord.com/developers/resources/audit-log).
