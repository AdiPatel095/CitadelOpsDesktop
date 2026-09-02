import type { WorldIntelligenceEventScoreObservationV1 } from '../../api/Contracts';

/**
 * Returns one final known score for each completed event occurrence.
 *
 * The backend keeps a latest row per occurrence and leaderboard. A player can
 * therefore have duplicate rows for the same run after moving leagues or
 * appearing on more than one board. Previous scores are run-shaped, so the
 * newest known row wins and active occurrences stay in the live views.
 */
export function completedEventScoreFinals(
	entries: readonly WorldIntelligenceEventScoreObservationV1[],
	now = Date.now(),
): WorldIntelligenceEventScoreObservationV1[] {
	const byOccurrence = new Map<string, WorldIntelligenceEventScoreObservationV1>();
	for (const entry of entries) {
		const endsAt = Date.parse(entry.eventEndsAt);
		if (!entry.scoreKnown || typeof entry.score !== 'number' || !Number.isFinite(entry.score) || !Number.isFinite(endsAt) || endsAt > now) continue;
		const occurrence = entry.occurrenceId.trim();
		if (!occurrence) continue;
		const current = byOccurrence.get(occurrence);
		if (!current || preferFinalScore(entry, current)) byOccurrence.set(occurrence, entry);
	}
	return [...byOccurrence.values()].sort((left, right) => (
		Date.parse(right.eventEndsAt) - Date.parse(left.eventEndsAt)
		|| observedAt(right) - observedAt(left)
		|| eventLabel(left).localeCompare(eventLabel(right))
		|| left.occurrenceId.localeCompare(right.occurrenceId)
	));
}

/** Returns the next known end boundary so an open dossier can reveal it. */
export function nextEventScoreEnd(
	entries: readonly WorldIntelligenceEventScoreObservationV1[],
	now = Date.now(),
): number | null {
	let next = Number.POSITIVE_INFINITY;
	for (const entry of entries) {
		if (!entry.scoreKnown || typeof entry.score !== 'number' || !Number.isFinite(entry.score)) continue;
		const endsAt = Date.parse(entry.eventEndsAt);
		if (Number.isFinite(endsAt) && endsAt > now && endsAt < next) next = endsAt;
	}
	return Number.isFinite(next) ? next : null;
}

/** Formats the authoritative UTC end instant in the viewer's local zone. */
export function formatEventEndLocal(value: string, locales?: Intl.LocalesArgument): string {
	const date = new Date(value);
	return Number.isNaN(date.getTime())
		? 'Unknown'
		: date.toLocaleString(locales, { dateStyle: 'medium', timeStyle: 'short' });
}

function preferFinalScore(
	candidate: WorldIntelligenceEventScoreObservationV1,
	current: WorldIntelligenceEventScoreObservationV1,
): boolean {
	const candidateObservedAt = observedAt(candidate);
	const currentObservedAt = observedAt(current);
	if (candidateObservedAt !== currentObservedAt) return candidateObservedAt > currentObservedAt;
	if (candidate.score !== current.score) return (candidate.score ?? 0) > (current.score ?? 0);
	return `${candidate.listType}:${candidate.leagueId}` < `${current.listType}:${current.leagueId}`;
}

function observedAt(entry: WorldIntelligenceEventScoreObservationV1): number {
	const timestamp = Date.parse(entry.observedAt);
	return Number.isFinite(timestamp) ? timestamp : Number.NEGATIVE_INFINITY;
}

function eventLabel(entry: WorldIntelligenceEventScoreObservationV1): string {
	return entry.eventName.trim() || entry.eventKey.trim();
}
