import type { WorldIntelligenceEventScoreObservationV1 } from '../../api/Contracts';

const stormEventId = 102;

/** Rebuild dated Storm ranks only when complete final scores contradict stored page-local ranks. */
export function normalizeStormSessionRanking(
	eventId: number,
	entries: readonly WorldIntelligenceEventScoreObservationV1[],
): WorldIntelligenceEventScoreObservationV1[] {
	const normalized = entries.map((entry) => ({ ...entry }));
	if (eventId !== stormEventId || normalized.length < 2) return normalized;

	const boardIndices = new Map<string, number[]>();
	for (let index = 0; index < normalized.length; index += 1) {
		const entry = normalized[index];
		const key = `${entry.listType}:${entry.boardKey ?? ''}:${entry.leagueId}`;
		const indices = boardIndices.get(key) ?? [];
		indices.push(index);
		boardIndices.set(key, indices);
	}

	for (const indices of boardIndices.values()) {
		if (indices.length < 2 || stormRanksMatchScores(indices.map((index) => normalized[index]))) continue;
		indices.sort((leftIndex, rightIndex) => compareStormScores(normalized[leftIndex], normalized[rightIndex]));
		indices.forEach((entryIndex, rankIndex) => {
			normalized[entryIndex].rank = rankIndex + 1;
		});
	}
	return normalized;
}

function stormRanksMatchScores(entries: readonly WorldIntelligenceEventScoreObservationV1[]): boolean {
	if (!entries.every((entry) => stormScore(entry) != null)) return true;
	const ordered = [...entries].sort((left, right) => left.rank - right.rank);
	const scoreByRank = new Map<number, number>();
	let priorRank = 0;
	let priorScore = Number.POSITIVE_INFINITY;
	for (const entry of ordered) {
		const score = stormScore(entry);
		if (score == null) continue;
		if (!Number.isInteger(entry.rank) || entry.rank < 1) return false;
		const sameRankScore = scoreByRank.get(entry.rank);
		if (sameRankScore != null && sameRankScore !== score) return false;
		if (entry.rank > priorRank && score > priorScore) return false;
		scoreByRank.set(entry.rank, score);
		priorRank = entry.rank;
		priorScore = score;
	}
	return scoreByRank.has(1);
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
