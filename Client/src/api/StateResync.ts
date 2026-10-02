/**
 * Revision bookkeeping for the dashboard state stream.
 *
 * The server sends one `state.changed` event per committed revision. When a
 * connection cannot keep up, the server merges its oldest pending event into the
 * newest one. The merged event has `gap: true`, a complete patch for everything
 * it covers, and a `baseRevision` naming the oldest revision the patch builds on.
 * The untouched middle events still follow it, so events may arrive with a base
 * the client has not reached yet and be bridged a moment later by the merged one.
 *
 * `StateResync` is a pure, clock-injected state machine (no timers, no I/O, no
 * React) that decides for every incoming event whether to
 *   - apply it (its base is at or below the current revision),
 *   - buffer it (its base is ahead of the current revision; a later event may
 *     bridge the hole), or
 *   - ignore it (its revision is not newer than the current state),
 * and asks for at most one full-state resync at a time when replay cannot bridge
 * a hole, the schema changed, or the buffer overflowed. The host owns transport
 * and timers: it performs the snapshot request when an outcome carries `resync`,
 * feeds the snapshot back with `acceptSnapshot`, and re-enters through `tick` at
 * the outcome's `wakeAt`. Events received while a snapshot is outstanding are
 * buffered and replayed (revision above the snapshot revision, in order) once it
 * lands, so no event is dropped. Requests are spaced by exponential backoff.
 *
 * This file is shared byte-for-byte between the desktop client and the hosted
 * command center; keep it free of imports.
 */

export interface VersionedState {
	revision: number;
	schemaVersion: number;
}

export interface VersionedPatch {
	revision: number;
	schemaVersion: number;
}

export interface StateEventInput<P extends VersionedPatch> {
	patch: P;
	/** Envelope `gap`; informational, the base revision decides applicability. */
	gap?: boolean;
	/** Envelope `baseRevision`; absent on older servers, where `revision - 1` is used. */
	baseRevision?: number;
}

export type ResyncReason = 'missing-state' | 'gap' | 'schema-change' | 'buffer-overflow';

export interface ResyncRequest {
	reason: ResyncReason;
	/** 1 for the first request of an episode; grows while requests keep failing to settle. */
	attempt: number;
	/**
	 * Where the host should ask. `socket` while the previous request (if any) was
	 * answered: the snapshot then arrives in order with later events and costs no
	 * REST egress. `rest` once a request went unanswered or failed, so a socket
	 * that swallows requests cannot stall recovery. The host may still downgrade
	 * `socket` to REST when the socket is not open.
	 */
	transport: 'socket' | 'rest';
}

export interface StateResyncOutcome<S> {
	state: S | null;
	/** True when `state` is a new object the host should publish. */
	changed: boolean;
	/** The host must start exactly one full-state request now. */
	resync: ResyncRequest | null;
	/** Absolute time (ms) at which the host should call `tick`, or null when nothing is waiting. */
	wakeAt: number | null;
}

export interface StateResyncOptions {
	/** How long an unbridged hole may wait for a covering event before a resync is requested. */
	gapGraceMs?: number;
	/** Upper bound on buffered events; beyond it the buffer is dropped and a resync requested. */
	maxBufferedEvents?: number;
	/** How long an outstanding snapshot request may stay unanswered. */
	resyncTimeoutMs?: number;
	/** Minimum spacing between request starts, doubled per consecutive attempt. */
	retryBaseMs?: number;
	retryMaxMs?: number;
	/** After this long without a request the attempt counter starts over. */
	quietResetMs?: number;
}

interface BufferedEvent<P extends VersionedPatch> {
	revision: number;
	base: number;
	patch: P;
}

export const defaultStateResyncOptions: Required<StateResyncOptions> = {
	gapGraceMs: 750,
	maxBufferedEvents: 256,
	resyncTimeoutMs: 15_000,
	retryBaseMs: 1_000,
	retryMaxMs: 30_000,
	quietResetMs: 60_000,
};

export class StateResync<S extends VersionedState, P extends VersionedPatch> {
	private state: S | null = null;
	private buffer: Array<BufferedEvent<P>> = [];
	private awaiting: { since: number; reason: ResyncReason } | null = null;
	private gapSince: number | null = null;
	private pendingReason: ResyncReason | null = null;
	private eventsLost = false;
	/** The last snapshot request timed out or failed and no snapshot has arrived since. */
	private unanswered = false;
	private attempt = 0;
	private lastRequestAt = Number.NEGATIVE_INFINITY;
	private nextRequestAt = Number.NEGATIVE_INFINITY;
	private readonly applyPatch: (state: S, patch: P) => S;
	private readonly options: Required<StateResyncOptions>;

	constructor(applyPatch: (state: S, patch: P) => S, options: StateResyncOptions = {}) {
		this.applyPatch = applyPatch;
		this.options = { ...defaultStateResyncOptions, ...options };
	}

	current(): S | null {
		return this.state;
	}

	/** True while a snapshot request is outstanding. */
	get resyncInFlight(): boolean {
		return this.awaiting != null;
	}

	/** Number of buffered events, for diagnostics and tests. */
	get bufferedEvents(): number {
		return this.buffer.length;
	}

	/** Accepts a snapshot from any source (socket greeting, `query.state` reply, REST). */
	acceptSnapshot(snapshot: S, now: number): StateResyncOutcome<S> {
		const current = this.state;
		if (current != null && current.revision > snapshot.revision) {
			// Older than what is already shown, for example a slow REST reply.
			return this.outcome(false, null, now);
		}
		this.state = snapshot;
		this.awaiting = null;
		this.unanswered = false;
		this.pendingReason = null;
		this.gapSince = null;
		const mismatch = this.drain(now);
		let resync: ResyncRequest | null = null;
		if (mismatch) {
			resync = this.request('schema-change', now);
		} else if (this.eventsLost) {
			// Events beyond the buffer bound were dropped while waiting; the snapshot may not cover them.
			this.eventsLost = false;
			resync = this.request('buffer-overflow', now);
		}
		return this.outcome(true, resync, now);
	}

	/** Feeds one `state.changed` event. */
	receiveEvent(event: StateEventInput<P>, now: number): StateResyncOutcome<S> {
		const patch = event.patch;
		const revision = patch.revision;
		const base = typeof event.baseRevision === 'number' && event.baseRevision >= 0 && event.baseRevision < revision
			? event.baseRevision
			: revision - 1;
		const current = this.state;

		if (current == null) {
			this.enqueue({ revision, base, patch });
			return this.outcome(false, this.awaiting == null ? this.request('missing-state', now) : null, now);
		}
		if (revision <= current.revision) return this.outcome(false, null, now);
		if (this.awaiting != null) {
			this.enqueue({ revision, base, patch });
			return this.outcome(false, null, now);
		}
		if (patch.schemaVersion !== current.schemaVersion) {
			this.enqueue({ revision, base, patch });
			return this.outcome(false, this.request('schema-change', now), now);
		}
		if (base > current.revision) {
			// A hole: the events between us and this one were merged into a later event, or lost.
			this.enqueue({ revision, base, patch });
			this.gapSince ??= now;
			return this.outcome(false, null, now);
		}
		this.state = this.applyPatch(current, patch);
		const mismatch = this.drain(now);
		return this.outcome(true, mismatch ? this.request('schema-change', now) : null, now);
	}

	/** An event arrived that could not be decoded: only a snapshot can repair the missing revision. */
	unusableEvent(now: number): StateResyncOutcome<S> {
		return this.outcome(false, this.request('gap', now), now);
	}

	/** Advances timers: grace expiry, request timeout, backoff release. */
	tick(now: number): StateResyncOutcome<S> {
		if (this.awaiting != null && now - this.awaiting.since >= this.options.resyncTimeoutMs) {
			this.pendingReason = this.awaiting.reason;
			this.awaiting = null;
			this.unanswered = true;
		}
		let resync: ResyncRequest | null = null;
		if (this.awaiting == null) {
			if (this.pendingReason != null) {
				resync = this.request(this.pendingReason, now);
			} else if (this.gapSince != null && now - this.gapSince >= this.options.gapGraceMs) {
				resync = this.request('gap', now);
			}
		}
		return this.outcome(false, resync, now);
	}

	/** The host could not deliver or complete the snapshot request. */
	resyncFailed(now: number): StateResyncOutcome<S> {
		if (this.awaiting != null) {
			this.pendingReason = this.awaiting.reason;
			this.awaiting = null;
			this.unanswered = true;
		}
		return this.tick(now);
	}

	/**
	 * The event connection was lost. A reconnect starts with a fresh snapshot, so
	 * the outstanding request and everything buffered for the old connection is
	 * void. The shown state stays until that snapshot arrives; backoff is kept.
	 */
	connectionReset(): void {
		this.awaiting = null;
		// A new socket has not failed to answer anything yet.
		this.unanswered = false;
		this.pendingReason = null;
		this.gapSince = null;
		this.eventsLost = false;
		this.buffer = [];
	}

	/** Discard state belonging to a different server instance, including lower revisions. */
	forgetState(): void {
		this.state = null;
		this.connectionReset();
	}

	private request(reason: ResyncReason, now: number): ResyncRequest | null {
		if (this.awaiting != null) return null;
		if (now < this.nextRequestAt) {
			this.pendingReason ??= reason;
			return null;
		}
		if (now - this.lastRequestAt > this.options.quietResetMs) this.attempt = 0;
		this.attempt += 1;
		this.lastRequestAt = now;
		const spacing = Math.min(this.options.retryMaxMs, this.options.retryBaseMs * 2 ** (this.attempt - 1));
		this.nextRequestAt = now + spacing;
		this.pendingReason = null;
		this.awaiting = { since: now, reason };
		return { reason, attempt: this.attempt, transport: this.unanswered ? 'rest' : 'socket' };
	}

	private enqueue(event: BufferedEvent<P>): void {
		const existing = this.buffer.findIndex((entry) => entry.revision === event.revision);
		if (existing >= 0) {
			// Same revision twice: keep whichever covers more.
			if (event.base < this.buffer[existing].base) this.buffer[existing] = event;
			return;
		}
		let index = this.buffer.length;
		while (index > 0 && this.buffer[index - 1].revision > event.revision) index -= 1;
		this.buffer.splice(index, 0, event);
		if (this.buffer.length > this.options.maxBufferedEvents) {
			this.buffer = [];
			this.gapSince = null;
			// Dropped while a snapshot is outstanding: that snapshot may predate them, so check after it lands.
			// Dropped otherwise: the snapshot requested next is taken after them and covers them.
			if (this.awaiting != null) this.eventsLost = true;
			else this.pendingReason = 'buffer-overflow';
		}
	}

	/**
	 * Replays buffered events that are now applicable, in revision order.
	 * Returns true when the next candidate needs a different schema than the state.
	 */
	private drain(now: number): boolean {
		let mismatch = false;
		for (;;) {
			const current = this.state;
			if (current == null) break;
			this.buffer = this.buffer.filter((entry) => entry.revision > current.revision);
			const next = this.buffer.find((entry) => entry.base <= current.revision);
			if (next == null) break;
			if (next.patch.schemaVersion !== current.schemaVersion) {
				mismatch = true;
				break;
			}
			this.state = this.applyPatch(current, next.patch);
		}
		if (this.buffer.length === 0) this.gapSince = null;
		else if (this.gapSince == null && !mismatch) this.gapSince = now;
		return mismatch;
	}

	private outcome(changed: boolean, resync: ResyncRequest | null, now: number): StateResyncOutcome<S> {
		return { state: this.state, changed, resync, wakeAt: this.nextWake(now) };
	}

	private nextWake(now: number): number | null {
		if (this.awaiting != null) return this.awaiting.since + this.options.resyncTimeoutMs;
		if (this.pendingReason != null) return Math.max(now, this.nextRequestAt);
		if (this.gapSince != null) return Math.max(this.gapSince + this.options.gapGraceMs, this.nextRequestAt);
		return null;
	}
}
