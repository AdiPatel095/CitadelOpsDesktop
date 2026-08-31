export function advanceSuccessfulLoadGeneration(
	appliedGeneration: number,
	candidateGeneration: number,
): number | null {
	return Number.isSafeInteger(candidateGeneration) && candidateGeneration > appliedGeneration
		? candidateGeneration
		: null;
}
