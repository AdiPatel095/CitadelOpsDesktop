import type { WorldIntelligenceEventScoreObservationV1 } from '../../api/Contracts';

export const stormEventId = 102;
export const stormListType = 13;
export const stormGlobalLeagueId = -1;

/** Storm list 13 is one world-wide ranking. Wire LID 1 is not a player level league. */
export function isOriginalStormRanking(eventId: number | undefined, listType: number | undefined): boolean {
	return eventId === stormEventId && listType === stormListType;
}

/** Collapse legacy league-shaped Storm rows and rebuild their one global score ranking. */
export function normalizeStormSessionRanking(
	eventId: number,
	entries: readonly WorldIntelligenceEventScoreObservationV1[],
): WorldIntelligenceEventScoreObservationV1[] {
	if (eventId !== stormEventId) return entries.map((entry) => ({ ...entry }));

	const normalized: WorldIntelligenceEventScoreObservationV1[] = [];
	const stormPlayerIndices = new Map<number, number>();
	for (const sourceEntry of entries) {
		if (!isOriginalStormRanking(sourceEntry.eventId, sourceEntry.listType)) {
			normalized.push({ ...sourceEntry });
			continue;
		}
		const canonical = { ...sourceEntry, leagueId: stormGlobalLeagueId };
		delete canonical.boardKey;
		const existingIndex = stormPlayerIndices.get(canonical.playerId);
		if (existingIndex == null) {
			stormPlayerIndices.set(canonical.playerId, normalized.length);
			normalized.push(canonical);
			continue;
		}
		if (preferStormObservation(canonical, normalized[existingIndex])) {
			normalized[existingIndex] = canonical;
		}
	}

	const stormIndices = [...stormPlayerIndices.values()];
	// Cargo is the authority for a complete Storm board. The game-provided
	// rank is display metadata and never controls inclusion or ordering.
	if (stormIndices.length >= 2 && stormIndices.every((index) => stormScore(normalized[index]) != null)) {
		stormIndices.sort((leftIndex, rightIndex) => compareStormScores(normalized[leftIndex], normalized[rightIndex]));
		stormIndices.forEach((entryIndex, rankIndex) => {
			normalized[entryIndex].rank = rankIndex + 1;
		});
	}
	return normalized;
}

function preferStormObservation(
	candidate: WorldIntelligenceEventScoreObservationV1,
	current: WorldIntelligenceEventScoreObservationV1,
): boolean {
	const candidateObservedAt = Date.parse(candidate.observedAt);
	const currentObservedAt = Date.parse(current.observedAt);
	if (Number.isFinite(candidateObservedAt) && Number.isFinite(currentObservedAt) && candidateObservedAt !== currentObservedAt) {
		return candidateObservedAt > currentObservedAt;
	}
	if (candidate.scoreKnown !== current.scoreKnown) return candidate.scoreKnown;
	const candidateScore = stormScore(candidate);
	const currentScore = stormScore(current);
	if (candidateScore != null && currentScore != null && candidateScore !== currentScore) return candidateScore > currentScore;
	return false;
}

function compareStormScores(
	left: WorldIntelligenceEventScoreObservationV1,
	right: WorldIntelligenceEventScoreObservationV1,
): number {
	const leftScore = stormScore(left);
	const rightScore = stormScore(right);
	if (leftScore == null && rightScore != null) return 1;
	if (leftScore != null && rightScore == null) return -1;
	if (leftScore != null && rightScore != null && leftScore !== rightScore) return rightScore - leftScore;
	return left.playerId - right.playerId;
}

function stormScore(entry: WorldIntelligenceEventScoreObservationV1): number | null {
	if (!entry.scoreKnown) return null;
	return typeof entry.score === 'number' && Number.isFinite(entry.score) ? entry.score : 0;
}
