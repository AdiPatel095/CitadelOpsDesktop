import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

// CIT-27: StateResync applies, buffers and resyncs the dashboard state stream without a reload loop.
// The same test runs in the desktop client and the hosted command center (only STATE_RESYNC differs).
const STATE_RESYNC = '../src/api/StateResync.ts';
const vite = await createServer({
	root: fileURLToPath(new URL('..', import.meta.url)),
	appType: 'custom',
	logLevel: 'silent',
	server: { middlewareMode: true },
});
after(() => vite.close());
const { StateResync } = await vite.ssrLoadModule(fileURLToPath(new URL(STATE_RESYNC, import.meta.url)));

// A toy state: `values[key]` is the latest revision that wrote it. Patches are complete for the keys they touch,
// exactly like the server's merged patches, so replaying any covering patch on an older-or-equal base is exact.
const applyPatch = (state, patch) => ({
	revision: patch.revision,
	schemaVersion: patch.schemaVersion,
	values: { ...state.values, ...patch.set },
});
const snapshotAt = (revision, schemaVersion = 1) => ({
	revision,
	schemaVersion,
	values: Object.fromEntries(Array.from({ length: revision }, (_, i) => [`k${i + 1}`, i + 1])),
});
// Plain event for one revision, writing key k<revision>.
const plain = (revision, schemaVersion = 1) => ({
	patch: { revision, schemaVersion, set: { [`k${revision}`]: revision } },
	baseRevision: revision - 1,
});
// Merged event covering (base, revision], the way the server coalesces a full subscriber buffer.
const merged = (base, revision, schemaVersion = 1) => ({
	patch: {
		revision,
		schemaVersion,
		set: Object.fromEntries(Array.from({ length: revision - base }, (_, i) => [`k${base + i + 1}`, base + i + 1])),
	},
	gap: true,
	baseRevision: base,
});

const newMachine = (options) => new StateResync(applyPatch, options);
const started = (machine, revision = 0, now = 0) => {
	machine.acceptSnapshot(snapshotAt(revision), now);
	return machine;
};
const assertState = (machine, revision) => {
	assert.deepEqual(machine.current(), snapshotAt(revision), `state must equal the server state at revision ${revision}`);
};

test('in-order events apply directly and never request a snapshot', () => {
	const machine = started(newMachine());
	for (let revision = 1; revision <= 50; revision += 1) {
		const outcome = machine.receiveEvent(plain(revision), revision * 10);
		assert.equal(outcome.changed, true);
		assert.equal(outcome.resync, null);
		assert.equal(outcome.wakeAt, null);
	}
	assertState(machine, 50);
	assert.equal(machine.bufferedEvents, 0);
});

test('a gap event with a declared base applies without a snapshot request', () => {
	const machine = started(newMachine(), 4);
	const outcome = machine.receiveEvent(merged(4, 9), 100);
	assert.equal(outcome.resync, null);
	assert.equal(outcome.changed, true);
	assertState(machine, 9);
});

test('a gap event whose base is below the current revision still applies (overlap is idempotent)', () => {
	const machine = started(newMachine(), 6);
	const outcome = machine.receiveEvent(merged(2, 9), 100);
	assert.equal(outcome.resync, null);
	assertState(machine, 9);
});

test('older servers: an event without baseRevision applies only on top of revision-1', () => {
	const machine = started(newMachine(), 3);
	const legacy = (revision, gap) => ({ ...plain(revision), baseRevision: undefined, gap });
	assert.equal(machine.receiveEvent(legacy(4, true), 10).resync, null);
	assertState(machine, 4);
	// Revision 6 with an unknown base cannot be trusted to build on 4: hole, then one resync after the grace period.
	const hole = machine.receiveEvent(legacy(6, true), 20);
	assert.equal(hole.resync, null);
	assert.equal(hole.wakeAt, 20 + 750);
	assert.equal(machine.tick(20 + 750).resync?.reason, 'gap');
});

test('stale and duplicate events are ignored', () => {
	const machine = started(newMachine(), 5);
	const before = machine.current();
	assert.equal(machine.receiveEvent(plain(5), 1).changed, false);
	assert.equal(machine.receiveEvent(plain(3), 1).changed, false);
	assert.equal(machine.current(), before);
});

test('events whose base is ahead of the state wait for a merged event that bridges the hole', () => {
	// Revisions 2..64 arrive first (each with a base the client has not reached), then merged(0 -> 65).
	// No snapshot may be requested for the hole, and the buffer is emptied by the bridge.
	const machine = started(newMachine());
	let requests = 0;
	for (let revision = 2; revision <= 64; revision += 1) {
		const outcome = machine.receiveEvent(plain(revision), 100 + revision);
		if (outcome.resync) requests += 1;
		assert.equal(outcome.changed, false);
	}
	assert.equal(machine.bufferedEvents, 63);
	const bridged = machine.receiveEvent(merged(0, 65), 200);
	assert.equal(bridged.changed, true);
	assert.equal(bridged.resync, null);
	assert.equal(requests, 0);
	assert.equal(machine.bufferedEvents, 0);
	assert.equal(bridged.wakeAt, null);
	assertState(machine, 65);
	// A merged event that overlaps what is applied already: base 1, revision 66.
	assert.equal(machine.receiveEvent(merged(1, 66), 210).resync, null);
	assertState(machine, 66);
});

test('a hole that is never bridged triggers exactly one resync after the grace period', () => {
	const machine = started(newMachine());
	assert.equal(machine.receiveEvent(plain(3), 1_000).wakeAt, 1_750);
	assert.equal(machine.receiveEvent(plain(4), 1_010).resync, null);
	assert.equal(machine.tick(1_500).resync, null);
	const fired = machine.tick(1_750);
	assert.deepEqual(fired.resync, { reason: 'gap', attempt: 1, transport: 'socket' });
	assert.equal(machine.resyncInFlight, true);
	// Further ticks and events while the snapshot is outstanding never ask again.
	assert.equal(machine.tick(1_800).resync, null);
	assert.equal(machine.receiveEvent(plain(5), 1_900).resync, null);
	assert.equal(machine.receiveEvent(plain(6), 1_950).resync, null);
});

test('events received during a resync are buffered and replayed above the snapshot revision', () => {
	const machine = started(newMachine(), 2);
	machine.receiveEvent(plain(5), 0);
	assert.equal(machine.tick(750).resync?.reason, 'gap');
	// Burst while the snapshot request is outstanding, out of order and partly older than the snapshot.
	for (const revision of [9, 6, 8, 7, 10, 11]) {
		const outcome = machine.receiveEvent(plain(revision), 800 + revision);
		assert.equal(outcome.changed, false);
		assert.equal(outcome.resync, null);
	}
	assert.equal(machine.bufferedEvents, 7);
	const landed = machine.acceptSnapshot(snapshotAt(7), 900);
	assert.equal(landed.changed, true);
	assert.equal(landed.resync, null);
	assert.equal(machine.resyncInFlight, false);
	assertState(machine, 11);
	assert.equal(machine.bufferedEvents, 0);
	assert.equal(landed.wakeAt, null);
});

test('replay that cannot bridge asks again, spaced by backoff, and stops once settled', () => {
	const machine = started(newMachine({ gapGraceMs: 100, retryBaseMs: 1_000 }), 0);
	machine.receiveEvent(plain(20), 0);
	assert.deepEqual(machine.tick(100).resync, { reason: 'gap', attempt: 1, transport: 'socket' });
	// The snapshot lands at revision 10, still short of the buffered revision 20 (base 19).
	const landed = machine.acceptSnapshot(snapshotAt(10), 150);
	assert.equal(landed.resync, null);
	assert.equal(landed.wakeAt, 1_100, 'grace restarted from the snapshot, but request starts are spaced 1000 ms apart');
	assert.equal(machine.tick(250).resync, null);
	const second = machine.tick(1_100);
	assert.deepEqual(second.resync, { reason: 'gap', attempt: 2, transport: 'socket' });
	// Second spacing doubles.
	machine.acceptSnapshot(snapshotAt(15), 1_150);
	assert.equal(machine.tick(1_300).resync, null);
	assert.equal(machine.tick(3_000).resync, null, 'still inside 2000 ms from the second start');
	const third = machine.tick(3_100);
	assert.deepEqual(third.resync, { reason: 'gap', attempt: 3, transport: 'socket' });
	// Finally the snapshot covers it.
	const settled = machine.acceptSnapshot(snapshotAt(20), 3_200);
	assert.equal(settled.wakeAt, null);
	assertState(machine, 20);
	assert.equal(machine.tick(60_000).resync, null);
});

test('backoff is capped and the attempt counter resets after a quiet period', () => {
	const machine = started(newMachine({ gapGraceMs: 0, retryBaseMs: 1_000, retryMaxMs: 4_000, quietResetMs: 60_000 }));
	let now = 0;
	const attempts = [];
	for (let i = 0; i < 6; i += 1) {
		machine.receiveEvent(plain(100 + i * 10), now);
		let request = machine.tick(now).resync;
		while (request == null) {
			now += 500;
			request = machine.tick(now).resync;
		}
		attempts.push(request.attempt);
		machine.acceptSnapshot(snapshotAt(100 + i * 10 - 1), now + 10);
		now += 10;
	}
	assert.deepEqual(attempts, [1, 2, 3, 4, 5, 6]);
	// Spacing never exceeds the cap: the next request starts within 4000 ms + tick granularity.
	machine.receiveEvent(plain(400), now);
	const before = now;
	let request = machine.tick(now).resync;
	while (request == null) {
		now += 500;
		request = machine.tick(now).resync;
	}
	assert.ok(now - before <= 4_500, `waited ${now - before} ms`);
	machine.acceptSnapshot(snapshotAt(399), now);
	now += 61_000;
	machine.receiveEvent(plain(500), now);
	assert.equal(machine.tick(now).resync?.attempt, 1, 'a quiet minute starts a fresh episode');
});

test('a snapshot request that goes unanswered times out and is retried', () => {
	const machine = started(newMachine({ gapGraceMs: 0, resyncTimeoutMs: 5_000, retryBaseMs: 1_000 }));
	machine.receiveEvent(plain(9), 0);
	assert.deepEqual(machine.tick(0).resync, { reason: 'gap', attempt: 1, transport: 'socket' });
	assert.equal(machine.tick(4_999).resync, null);
	const retried = machine.tick(5_000);
	assert.deepEqual(retried.resync, { reason: 'gap', attempt: 2, transport: 'rest' });
	machine.acceptSnapshot(snapshotAt(9), 5_100);
	assertState(machine, 9);
});

test('transport failure schedules a backed-off retry instead of retrying immediately', () => {
	const machine = started(newMachine({ gapGraceMs: 0, retryBaseMs: 1_000 }));
	machine.receiveEvent(plain(9), 0);
	assert.ok(machine.tick(0).resync);
	const failed = machine.resyncFailed(10);
	assert.equal(failed.resync, null);
	assert.equal(failed.wakeAt, 1_000);
	assert.equal(machine.tick(999).resync, null);
	assert.equal(machine.tick(1_000).resync?.attempt, 2);
});

test('a schema change requests one resync immediately and replays events of the new schema afterwards', () => {
	const machine = started(newMachine(), 3);
	const first = machine.receiveEvent(plain(4, 2), 0);
	assert.deepEqual(first.resync, { reason: 'schema-change', attempt: 1, transport: 'socket' });
	assert.equal(first.changed, false);
	assert.equal(machine.receiveEvent(plain(5, 2), 5).resync, null);
	assert.equal(machine.receiveEvent(plain(6, 2), 6).resync, null);
	assertState(machine, 3);
	const landed = machine.acceptSnapshot(snapshotAt(4, 2), 50);
	assert.equal(landed.resync, null);
	assert.deepEqual(machine.current(), { revision: 6, schemaVersion: 2, values: { ...snapshotAt(4).values, k5: 5, k6: 6 } });
});

test('a snapshot with a schema older than the buffered events asks once more, backed off', () => {
	const machine = started(newMachine(), 3);
	machine.receiveEvent(plain(4, 2), 0);
	const landed = machine.acceptSnapshot(snapshotAt(3, 1), 10);
	assert.equal(landed.resync, null, 'blocked by backoff spacing');
	assert.equal(landed.wakeAt, 1_000);
	assert.equal(machine.tick(1_000).resync?.reason, 'schema-change');
});

test('before any state, events are buffered behind a single snapshot request', () => {
	const machine = newMachine();
	const first = machine.receiveEvent(plain(4), 0);
	assert.deepEqual(first.resync, { reason: 'missing-state', attempt: 1, transport: 'socket' });
	assert.equal(machine.receiveEvent(plain(5), 1).resync, null);
	machine.acceptSnapshot(snapshotAt(3), 10);
	assertState(machine, 5);
});

test('a stale snapshot never replaces newer state or cancels an outstanding request', () => {
	const machine = started(newMachine({ gapGraceMs: 0 }), 10);
	machine.receiveEvent(plain(20), 0);
	assert.ok(machine.tick(0).resync);
	const stale = machine.acceptSnapshot(snapshotAt(4), 5);
	assert.equal(stale.changed, false);
	assert.equal(machine.resyncInFlight, true);
	assert.equal(machine.current().revision, 10);
});

test('buffer overflow drops the buffer and requests one snapshot', () => {
	const machine = started(newMachine({ maxBufferedEvents: 4, gapGraceMs: 10_000 }));
	let outcome;
	for (let revision = 10; revision <= 14; revision += 1) outcome = machine.receiveEvent(plain(revision), revision);
	assert.equal(machine.bufferedEvents, 0);
	assert.equal(outcome.wakeAt, 14, 'resync is pending and due now');
	assert.deepEqual(machine.tick(14).resync, { reason: 'buffer-overflow', attempt: 1, transport: 'socket' });
	machine.acceptSnapshot(snapshotAt(14), 20);
	assertState(machine, 14);
});

test('events lost to overflow while a snapshot is outstanding trigger a follow-up resync', () => {
	const machine = started(newMachine({ maxBufferedEvents: 3, gapGraceMs: 0, retryBaseMs: 100 }));
	machine.receiveEvent(plain(5), 0);
	assert.ok(machine.tick(0).resync);
	for (let revision = 6; revision <= 8; revision += 1) machine.receiveEvent(plain(revision), 1);
	assert.equal(machine.bufferedEvents, 0);
	const landed = machine.acceptSnapshot(snapshotAt(5), 50);
	assert.equal(landed.resync, null, 'spacing');
	assert.equal(machine.tick(100).resync?.reason, 'buffer-overflow');
});

test('connection reset forgets the old socket but keeps the shown state and the backoff', () => {
	const machine = started(newMachine({ gapGraceMs: 0, retryBaseMs: 1_000 }), 7);
	machine.receiveEvent(plain(12), 0);
	assert.ok(machine.tick(0).resync);
	machine.receiveEvent(plain(13), 1);
	machine.connectionReset();
	assert.equal(machine.resyncInFlight, false);
	assert.equal(machine.bufferedEvents, 0);
	assert.equal(machine.current().revision, 7);
	assert.equal(machine.tick(500).resync, null, 'nothing pending');
	// The reconnect greeting is just a snapshot.
	machine.acceptSnapshot(snapshotAt(13), 600);
	assertState(machine, 13);
});

test('an undecodable event requests a snapshot once, rate limited', () => {
	const machine = started(newMachine({ retryBaseMs: 1_000 }), 3);
	assert.deepEqual(machine.unusableEvent(0).resync, { reason: 'gap', attempt: 1, transport: 'socket' });
	assert.equal(machine.unusableEvent(10).resync, null);
});

test('sustained fast streams with occasional merged events stay at zero snapshot requests', () => {
	const machine = started(newMachine());
	let revision = 0;
	let requests = 0;
	const step = (outcome) => {
		if (outcome.resync) requests += 1;
	};
	for (let round = 0; round < 40; round += 1) {
		for (let i = 0; i < 5; i += 1) {
			revision += 1;
			step(machine.receiveEvent(plain(revision), revision));
		}
		// Overflow episode: the server folds the whole queue (four events) into one merged event.
		const tail = revision + 4;
		step(machine.receiveEvent(merged(revision, tail), tail));
		revision = tail;
	}
	assert.equal(requests, 0);
	assertState(machine, revision);
});

// Mirrors Server/State/Store.go publish/coalesceEventQueue: a full subscriber buffer folds every queued event into the
// new one (oldest base, newest revision, union of touched keys) and queues that single event.
function simulateSubscriber({ capacity, commits, seed }) {
	let random = seed;
	const next = () => {
		random = (random * 1664525 + 1013904223) >>> 0;
		return random / 2 ** 32;
	};
	const machine = started(newMachine({ gapGraceMs: 750 }), 0, 0);
	const queue = [];
	let now = 0;
	let requests = 0;
	let coalesced = 0;
	const deliver = (event) => {
		const outcome = machine.receiveEvent(event, now);
		if (outcome.resync) requests += 1;
	};
	const publish = (event) => {
		if (queue.length < capacity) {
			queue.push(event);
			return;
		}
		const folded = [...queue.splice(0), event];
		coalesced += 1;
		queue.push({
			patch: {
				revision: event.patch.revision,
				schemaVersion: 1,
				set: Object.assign({}, ...folded.map((entry) => entry.patch.set)),
			},
			gap: true,
			baseRevision: folded[0].baseRevision,
		});
	};
	for (let revision = 1; revision <= commits; revision += 1) {
		publish(plain(revision));
		now += 10;
		// The writer is sometimes stalled (slow socket) and sometimes catches up in a burst.
		const drain = next() < 0.4 ? 0 : Math.floor(next() * 4);
		for (let i = 0; i < drain && queue.length > 0; i += 1) deliver(queue.shift());
	}
	while (queue.length > 0) deliver(queue.shift());
	return { machine, requests, coalesced };
}

test('against the server coalescing model, a stalled writer never causes a snapshot request', () => {
	for (const capacity of [1, 2, 3, 8, 64]) {
		for (let seed = 1; seed <= 25; seed += 1) {
			const { machine, requests, coalesced } = simulateSubscriber({ capacity, commits: 2_000, seed });
			assert.equal(requests, 0, `capacity ${capacity} seed ${seed}`);
			assert.equal(machine.bufferedEvents, 0, `capacity ${capacity} seed ${seed}`);
			assert.equal(machine.resyncInFlight, false);
			assertState(machine, 2_000);
			if (capacity <= 8) assert.ok(coalesced > 0, 'the scenario must actually exercise coalescing');
		}
	}
});

// CIT-27 D1: REST is for a socket that is down or did not answer, not for "the second resync within 60 s".
test('repeated resyncs within the backoff window keep using the socket while each is answered', () => {
	const machine = started(newMachine());
	let now = 100;
	for (let episode = 1; episode <= 6; episode += 1) {
		const outcome = machine.receiveEvent(plain(episode * 10 + 1000), now); // a hole: base far ahead
		assert.equal(outcome.resync, null, 'a hole waits for a bridging event first');
		const woke = machine.tick(now + 800);
		assert.ok(woke.resync, `episode ${episode} must request a snapshot`);
		assert.equal(woke.resync.transport, 'socket', `episode ${episode} (attempt ${woke.resync.attempt}) must ask over the socket`);
		assert.equal(woke.resync.attempt, episode, 'each episode is within 60 s of the last request, so the attempt counter keeps growing');
		machine.acceptSnapshot(snapshotAt(episode * 10 + 1000), now + 810);
		// Inside the 60 s quiet window (the attempt counter keeps growing) and past the 30 s backoff cap.
		now += 35_000;
	}
	assert.ok(now < 100 + 6 * 60_000, 'the episodes stayed within repeated-resync territory');
});

test('an unanswered socket request makes the retry use REST, and an answered REST snapshot returns to the socket', () => {
	const machine = started(newMachine({ resyncTimeoutMs: 1_000, retryBaseMs: 100 }));
	machine.receiveEvent(plain(50), 0);
	const first = machine.tick(750);
	assert.equal(first.resync.transport, 'socket');
	const timedOut = machine.tick(750 + 1_000);
	assert.ok(timedOut.resync, 'the timed-out request is retried');
	assert.equal(timedOut.resync.transport, 'rest', 'the socket did not answer, so the retry goes over REST');
	// REST fails too: still REST.
	machine.resyncFailed(2_000);
	const again = machine.tick(60_000);
	assert.equal(again.resync?.transport, 'rest');
	// The REST snapshot lands: the next episode is back on the socket.
	machine.acceptSnapshot(snapshotAt(50), 61_000);
	machine.receiveEvent(plain(9_000), 62_000);
	const next = machine.tick(62_000 + 800);
	assert.equal(next.resync?.transport, 'socket');
});

test('a delivery failure requests REST and a fresh connection starts on the socket again', () => {
	const machine = newMachine({ retryBaseMs: 10 });
	const first = machine.receiveEvent(plain(1), 0);
	assert.equal(first.resync?.reason, 'missing-state');
	assert.equal(first.resync?.transport, 'socket');
	const failed = machine.resyncFailed(5);
	assert.equal(failed.resync, null, 'backoff holds the retry');
	const retry = machine.tick(1_000);
	assert.equal(retry.resync?.transport, 'rest', 'the host could not deliver the socket request');
	machine.connectionReset();
	const afterReconnect = machine.receiveEvent(plain(2), 5_000);
	assert.equal(afterReconnect.resync?.transport, 'socket', 'a new connection has not failed to answer anything');
});

test('forgetState clears old-instance buffers and requests before accepting a lower snapshot', () => {
	const machine = started(newMachine(), 10);
	machine.receiveEvent(plain(12), 100);
	machine.tick(1_000);
	assert.equal(machine.resyncInFlight, true);
	assert.equal(machine.bufferedEvents, 1);
	machine.forgetState();
	assert.equal(machine.current(), null);
	assert.equal(machine.resyncInFlight, false);
	assert.equal(machine.bufferedEvents, 0);
	assert.equal(machine.tick(1_001).resync, null);
	assert.equal(machine.acceptSnapshot(snapshotAt(2), 1_002).changed, true);
	assertState(machine, 2);
});
