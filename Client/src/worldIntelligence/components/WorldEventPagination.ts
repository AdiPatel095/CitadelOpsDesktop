export interface EventPaginationSelection {
	worldId: string;
	eventKey: string;
	runKey: string;
	boardKey: string;
	league: string;
	searchQuery: string;
	sortKey: string;
	sortDirection: string;
}

export function eventBoardSelectionKey(
	worldId: string,
	eventKey: string,
	runKey: string,
	boardKey: string,
): string {
	return JSON.stringify([worldId.trim().toLocaleLowerCase(), eventKey, runKey, boardKey]);
}

/** Stable identity for the selected board and user-controlled filters. */
export function eventPaginationSelectionKey(selection: EventPaginationSelection): string {
	return JSON.stringify([
		selection.worldId.trim().toLocaleLowerCase(),
		selection.eventKey,
		selection.runKey,
		selection.boardKey,
		selection.league,
		selection.searchQuery.trim().toLocaleLowerCase(),
		selection.sortKey,
		selection.sortDirection,
	]);
}

/** Keep the current page for same-board updates and clamp only when rows contract. */
export function reconcileEventPage(
	currentPage: number,
	previousSelectionKey: string,
	nextSelectionKey: string,
	totalRows: number,
	pageSize: number,
): number {
	if (previousSelectionKey !== nextSelectionKey) return 0;
	const boundedPageSize = Math.max(1, Math.trunc(pageSize));
	const pageCount = Math.max(1, Math.ceil(Math.max(0, totalRows) / boundedPageSize));
	return Math.max(0, Math.min(Math.trunc(currentPage), pageCount - 1));
}
