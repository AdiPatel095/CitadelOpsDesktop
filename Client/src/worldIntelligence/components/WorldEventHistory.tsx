import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowDown, ArrowUp, ArrowUpDown, CalendarDays, ChevronLeft, ChevronRight, History, RefreshCw, Search, Trophy } from 'lucide-react';
import { CitadelAPI, type WorldIntelligenceSubscriptionStatus } from '../../api/CitadelClient';
import type {
	WorldIntelligenceEventRunV1,
	WorldIntelligenceEventRunRankingV1,
	WorldIntelligenceEventRunRankingDeltaV1,
	WorldIntelligenceEventRunRankingSnapshotV1,
	WorldIntelligenceEventScoreObservationV1,
	WorldIntelligencePlayerEventScoreHistoryV1,
	WorldIntelligenceRankingEntryV1,
	WorldIntelligenceRankingResponseV1,
	WorldIntelligenceUpdateManifestV1,
} from '../../api/Contracts';
import { Badge, Button, Card, CardContent, EmptyState, Input, MetricTile, Select } from '../../components/ui';
import {
	eventBoardSelectionKey,
	eventPaginationSelectionKey,
	reconcileEventPage,
} from './WorldEventPagination';
import { advanceSuccessfulLoadGeneration } from './WorldIntelligenceLoadGeneration';
import {
	isOriginalStormRanking,
	normalizeStormSessionRanking,
	stormGlobalLeagueId,
} from './StormRanking';

const eventPageSize = 25;
const metadataRefreshInterval = 30_000;
const allEvents = '__all_events__';
const allLeagues = '__all_leagues__';
const regularRankingMetrics = ['might', 'honor'] as const;

type RegularRankingMetric = (typeof regularRankingMetrics)[number];
type ReferenceRankingKey = RegularRankingMetric | 'storm';
type ReferenceRankingSnapshots = Partial<Record<ReferenceRankingKey, WorldIntelligenceRankingResponseV1>>;

type LeagueDefinition = { eventId: number; leagueId: number; minimumLevel: number; maximumLevel: number };
type EventLeaderboardRow = {
	worldId: string;
	eventId: number;
	listType: number;
	leagueId: number;
	boardKey?: string;
	playerId: number;
	playerName: string;
	allianceId?: number;
	allianceName?: string;
	rank: number;
	score?: number;
	scoreKnown: boolean;
	scoreUnit: string;
	observedAt: string;
};
type EventBoard = {
	key: string;
	eventId: number;
	eventName: string;
	listType: number;
	boardKey: string;
	entries: EventLeaderboardRow[];
	run?: WorldIntelligenceEventRunV1;
};
type EventRunGroup = {
	key: string;
	title: string;
	runs: WorldIntelligenceEventRunV1[];
	publicBoard?: EventBoard;
};
type CachedRunBoards = { revision: number; lastObservedAt: string; response: WorldIntelligenceEventRunRankingV1 };
type EventSortKey = 'name' | 'might' | 'honor' | 'alliance' | 'rank' | 'score';
type EventSort = { key: EventSortKey; direction: 'ascending' | 'descending' };

const stormBoard = {
	eventId: 102,
	eventName: 'Storm Islands',
	listType: 13,
	leagueId: stormGlobalLeagueId,
	metric: 'public:storm-cargo-points',
};
const stormRunKey = `public-metric:${stormBoard.metric}`;

interface WorldEventHistoryProps {
	worldId: string;
	eventRuns?: number;
	eventScores?: number;
	currentPlayerId?: number;
	currentLeagueByEvent?: Record<number, number>;
	worldUpdate?: WorldIntelligenceUpdateManifestV1 | null;
	updateStreamConnected?: boolean;
	onOpenPlayer: (playerId: number, worldId: string) => void;
	onOpenAlliance: (allianceId: number, worldId: string) => void;
}

export const WorldEventHistory = ({
	worldId,
	eventRuns = 0,
	eventScores = 0,
	currentPlayerId,
	currentLeagueByEvent,
	worldUpdate,
	updateStreamConnected = false,
	onOpenPlayer,
	onOpenAlliance,
}: WorldEventHistoryProps) => {
	const [runs, setRuns] = useState<WorldIntelligenceEventRunV1[]>([]);
	const [runBoards, setRunBoards] = useState<Record<string, CachedRunBoards>>({});
	const [stormPublicBoard, setStormPublicBoard] = useState<EventBoard | null>(null);
	const [event, setEvent] = useState('');
	const [run, setRun] = useState('');
	const [board, setBoard] = useState('');
	const [league, setLeague] = useState(allLeagues);
	const [leagueDefinitions, setLeagueDefinitions] = useState<LeagueDefinition[]>([]);
	const [regularPlayers, setRegularPlayers] = useState<Map<number, WorldIntelligenceRankingEntryV1>>(new Map());
	const [searchQuery, setSearchQuery] = useState('');
	const [sort, setSort] = useState<EventSort>({ key: 'rank', direction: 'ascending' });
	const [page, setPage] = useState(0);
	const [directoryLoading, setDirectoryLoading] = useState(false);
	const [boardLoading, setBoardLoading] = useState(false);
	const [boardStreamStatus, setBoardStreamStatus] = useState<WorldIntelligenceSubscriptionStatus>('connecting');
	const [boardRefreshToken, setBoardRefreshToken] = useState(0);
	const [error, setError] = useState('');
	const directoryRequest = useRef(0);
	const runMetadataRequest = useRef(0);
	const runMetadataApplied = useRef(0);
	const referenceRankingsRequest = useRef(0);
	const regularRankingsApplied = useRef<Record<RegularRankingMetric, number>>({ might: 0, honor: 0 });
	const stormRankingsApplied = useRef(0);
	const referenceRankingSnapshots = useRef<ReferenceRankingSnapshots>({});
	const boardSubscriptionRequest = useRef(0);
	const boardFallbackRequest = useRef(0);
	const metadataRefreshInFlight = useRef(false);
	const previousWorldUpdate = useRef<WorldIntelligenceUpdateManifestV1 | null>(null);

	const loadRunMetadata = useCallback(async (requestID: number) => {
		const loadID = ++runMetadataRequest.current;
		const response = await CitadelAPI.getWorldIntelligenceEventRuns({ worldId, limit: 250 });
		const applied = advanceSuccessfulLoadGeneration(runMetadataApplied.current, loadID);
		if (requestID === directoryRequest.current && applied != null) {
			runMetadataApplied.current = applied;
			setRuns(orderedEventRuns(response.runs ?? []));
		}
	}, [worldId]);

	const loadReferenceRankings = useCallback(async (requestID: number) => {
		const loadID = ++referenceRankingsRequest.current;
		const [regularResponses, stormResponse] = await Promise.all([
			Promise.allSettled(regularRankingMetrics.map((metric) => (
				CitadelAPI.getWorldIntelligenceRankings({ worldId, type: 'players', metric, limit: 250 })
			))),
			CitadelAPI.getWorldIntelligenceRankings({ worldId, type: 'players', metric: stormBoard.metric, limit: 5_000 })
				.then((response) => ({ response, available: true as const }))
				.catch(() => ({ response: null, available: false as const })),
		]);
		if (requestID !== directoryRequest.current) return;
		let referencesChanged = false;
		for (const [index, result] of regularResponses.entries()) {
			if (result.status !== 'fulfilled') continue;
			const metric = regularRankingMetrics[index];
			const applied = advanceSuccessfulLoadGeneration(regularRankingsApplied.current[metric], loadID);
			if (applied == null) continue;
			regularRankingsApplied.current[metric] = applied;
			referenceRankingSnapshots.current[metric] = result.value;
			referencesChanged = true;
		}
		const stormApplied = stormResponse.available
			? advanceSuccessfulLoadGeneration(stormRankingsApplied.current, loadID)
			: null;
		if (stormApplied != null && stormResponse.response != null) {
			stormRankingsApplied.current = stormApplied;
			referenceRankingSnapshots.current.storm = stormResponse.response;
			referencesChanged = true;
			setStormPublicBoard(stormPublicRankingBoard(stormResponse.response));
		}
		if (referencesChanged) {
			setRegularPlayers(mergeRankingPlayers(referenceRankingSnapshots.current));
		}
	}, [worldId]);

	const refreshDirectory = useCallback(async () => {
		const requestID = ++directoryRequest.current;
		if (!worldId) {
			setRuns([]);
			setRegularPlayers(new Map());
			setStormPublicBoard(null);
			setDirectoryLoading(false);
			return;
		}
		setDirectoryLoading(true);
		setError('');
		try {
			await Promise.all([loadRunMetadata(requestID), loadReferenceRankings(requestID)]);
		} catch (requestError) {
			if (requestID === directoryRequest.current) {
				setError(errorMessage(requestError, 'Could not load event directory.'));
			}
		} finally {
			if (requestID === directoryRequest.current) setDirectoryLoading(false);
		}
	}, [loadReferenceRankings, loadRunMetadata, worldId]);

	useEffect(() => {
		setEvent('');
		setRun('');
		setBoard('');
		setRuns([]);
		setRunBoards({});
		setLeague(allLeagues);
		setPage(0);
		setRegularPlayers(new Map());
		setStormPublicBoard(null);
		referenceRankingSnapshots.current = {};
		previousWorldUpdate.current = null;
		void refreshDirectory();
	}, [refreshDirectory]);

	useEffect(() => {
		if (!worldId || updateStreamConnected) return;
		const refreshMetadata = async () => {
			if (metadataRefreshInFlight.current) return;
			metadataRefreshInFlight.current = true;
			const requestID = directoryRequest.current;
			try {
				await Promise.all([loadRunMetadata(requestID), loadReferenceRankings(requestID)]);
			} catch {
				// Preserve the last usable directory; the manual refresh surfaces errors.
			} finally {
				metadataRefreshInFlight.current = false;
			}
		};
		const interval = window.setInterval(() => void refreshMetadata(), metadataRefreshInterval);
		return () => window.clearInterval(interval);
	}, [loadReferenceRankings, loadRunMetadata, updateStreamConnected, worldId]);

	useEffect(() => {
		if (!worldUpdate || worldUpdate.worldId !== normalizeWorldID(worldId)) return;
		const previous = previousWorldUpdate.current;
		previousWorldUpdate.current = worldUpdate;
		const requestID = directoryRequest.current;
		// The ready manifest closes the race between the initial REST reads and
		// stream establishment. Generation guards suppress any older response.
		if (!previous || worldUpdate.eventRunsRevision > previous.eventRunsRevision) {
			void loadRunMetadata(requestID).catch(() => undefined);
		}
		if (!previous || worldUpdate.rankingsRevision > previous.rankingsRevision) {
			void loadReferenceRankings(requestID).catch(() => undefined);
		}
	}, [loadReferenceRankings, loadRunMetadata, worldId, worldUpdate]);

	useEffect(() => {
		void CitadelAPI.getWorldIntelligenceCatalogDataset('leaguetypes', 1)
			.then((dataset) => setLeagueDefinitions(parseLeagueDefinitions(dataset.rows)))
			.catch(() => setLeagueDefinitions([]));
	}, []);

	const eventGroups = useMemo(() => groupEventRuns(runs, stormPublicBoard), [runs, stormPublicBoard]);
	const selectedEventKey = eventGroups.some((candidate) => candidate.key === event)
		? event
		: eventGroups[0]?.key ?? '';
	const selectedEvent = eventGroups.find((candidate) => candidate.key === selectedEventKey) ?? null;
	const defaultRunKey = selectedEvent?.publicBoard
		? stormRunKey
		: selectedEvent?.runs[0]?.occurrenceId ?? '';
	const selectedRunKey = selectedEvent?.runs.some((candidate) => candidate.occurrenceId === run)
		? run
		: run === stormRunKey && selectedEvent?.publicBoard
			? stormRunKey
			: defaultRunKey;
	const selectedRun = selectedEvent?.runs.find((candidate) => candidate.occurrenceId === selectedRunKey) ?? null;
	const cachedRun = selectedRun ? runBoards[selectedRun.occurrenceId] : undefined;
	const selectedRunObservedAt = selectedRun?.lastObservedAt ?? '';
	const cachedRunObservedAt = cachedRun?.lastObservedAt ?? '';

	useEffect(() => {
		const subscriptionID = ++boardSubscriptionRequest.current;
		if (!worldId || !selectedRunKey || selectedRunKey === stormRunKey) {
			setBoardStreamStatus('connected');
			setBoardLoading(false);
			return;
		}
		setBoardStreamStatus('connecting');
		setBoardLoading(true);
		setError('');
		const unsubscribe = CitadelAPI.subscribeWorldIntelligenceLeaderboard(
			{ worldId, occurrenceId: selectedRunKey },
			(snapshot: WorldIntelligenceEventRunRankingSnapshotV1) => {
				if (subscriptionID !== boardSubscriptionRequest.current) return;
				setRunBoards((current) => {
					const existing = current[selectedRunKey];
					if (existing && (existing.revision > snapshot.revision || (
						existing.revision === snapshot.revision
						&& observationAtLeast(existing.lastObservedAt, snapshot.run.lastObservedAt)
					))) return current;
					return {
						...current,
						[selectedRunKey]: {
							revision: snapshot.revision,
							lastObservedAt: snapshot.run.lastObservedAt,
							response: { run: snapshot.run, entries: snapshot.entries },
						},
					};
				});
				setBoardLoading(false);
				setError('');
			},
			(delta: WorldIntelligenceEventRunRankingDeltaV1) => {
				if (subscriptionID !== boardSubscriptionRequest.current) return;
				setRunBoards((current) => {
					const existing = current[selectedRunKey];
					if (!existing || delta.revision <= existing.revision) return current;
					const response = applyEventRunRankingDelta(existing.response, delta);
					return {
						...current,
						[selectedRunKey]: {
							revision: delta.revision,
							lastObservedAt: delta.run.lastObservedAt,
							response,
						},
					};
				});
				setBoardLoading(false);
			},
			(status) => {
				if (subscriptionID === boardSubscriptionRequest.current) setBoardStreamStatus(status);
			},
		);
		return () => {
			if (subscriptionID === boardSubscriptionRequest.current) boardSubscriptionRequest.current += 1;
			unsubscribe();
		};
	}, [boardRefreshToken, selectedRunKey, worldId]);

	useEffect(() => {
		const fallbackID = ++boardFallbackRequest.current;
		if (!worldId || !selectedRunKey || selectedRunKey === stormRunKey || boardStreamStatus !== 'reconnecting' ||
			cachedRunObservedAt === selectedRunObservedAt) return;
		let cancelled = false;
		setBoardLoading(true);
		setError('');
		void CitadelAPI.getWorldIntelligenceEventRunRankings({ worldId, occurrenceId: selectedRunKey, limit: 5_000 })
			.then((response) => {
				if (cancelled || fallbackID !== boardFallbackRequest.current) return;
				const responseObservedAt = response.run.lastObservedAt || selectedRunObservedAt;
				setRunBoards((current) => {
					const existing = current[selectedRunKey];
					if (existing && observationAtLeast(existing.lastObservedAt, responseObservedAt)) return current;
					return {
						...current,
						[selectedRunKey]: {
							revision: existing?.revision ?? 0,
							lastObservedAt: responseObservedAt,
							response,
						},
					};
				});
			})
			.catch((requestError) => {
				if (!cancelled && fallbackID === boardFallbackRequest.current) {
					setError(errorMessage(requestError, 'Could not load the selected event leaderboard.'));
				}
			})
			.finally(() => {
				if (!cancelled && fallbackID === boardFallbackRequest.current) setBoardLoading(false);
			});
		return () => { cancelled = true; };
	}, [boardStreamStatus, cachedRunObservedAt, selectedRunKey, selectedRunObservedAt, worldId]);

	const availableBoards = useMemo(() => (
		selectedRunKey === stormRunKey && selectedEvent?.publicBoard
			? [selectedEvent.publicBoard]
			: cachedRun ? eventBoardsFromRun(cachedRun.response) : []
	), [cachedRun, selectedEvent?.publicBoard, selectedRunKey]);
	const selectedBoardKey = availableBoards.some((candidate) => candidate.key === board)
		? board
		: availableBoards[0]?.key ?? '';
	const selectedBoard = availableBoards.find((candidate) => candidate.key === selectedBoardKey) ?? null;
	const eventOptions = eventGroups.map((candidate) => ({ value: candidate.key, label: candidate.title }));
	const runOptions = [
		...(selectedEvent?.publicBoard ? [{ value: stormRunKey, label: 'Live Storm metrics' }] : []),
		...(selectedEvent?.runs ?? []).map((candidate) => ({ value: candidate.occurrenceId, label: eventRunLabel(candidate) })),
	];
	const needsRunSelector = runOptions.length > 1;
	const boardOptions = availableBoards.map((candidate) => ({ value: candidate.key, label: eventBoardVariantLabel(candidate) }));
	const needsBoardSelector = boardOptions.length > 1;
	const boardEntries = useMemo(() => selectedBoard?.entries ?? [], [selectedBoard]);
	const originalStormSelected = isOriginalStormRanking(selectedBoard?.eventId, selectedBoard?.listType);
	const availableLeagueIds = useMemo(
		() => originalStormSelected
			? []
			: [...new Set(boardEntries.map((entry) => entry.leagueId))].sort((left, right) => left - right),
		[boardEntries, originalStormSelected],
	);
	const leagueOptions = useMemo(() => {
		return [
			{ value: allLeagues, label: 'All level leagues' },
			...availableLeagueIds.map((leagueId) => ({
				value: String(leagueId),
				label: levelLeagueLabel(selectedBoard?.eventId, leagueId, leagueDefinitions),
			})),
		];
	}, [availableLeagueIds, leagueDefinitions, selectedBoard?.eventId]);
	const needsLeagueSelector = !originalStormSelected && availableLeagueIds.length > 1;
	const livePlayerLeague = selectedBoard ? currentLeagueByEvent?.[selectedBoard.eventId] : undefined;
	const selectedBoardIdentity = eventBoardSelectionKey(worldId, selectedEventKey, selectedRunKey, selectedBoardKey);
	const previousBoardIdentity = useRef('');
	useEffect(() => {
		if (previousBoardIdentity.current === selectedBoardIdentity) return;
		previousBoardIdentity.current = selectedBoardIdentity;
		let defaultLeague = allLeagues;
		if (needsLeagueSelector) {
			const publicPlayerLeague = boardEntries.find((entry) => entry.playerId === currentPlayerId)?.leagueId;
			const preferredLeague = [livePlayerLeague, publicPlayerLeague]
				.find((leagueId) => leagueId != null && availableLeagueIds.includes(leagueId));
			if (preferredLeague != null) {
				defaultLeague = String(preferredLeague);
			}
		}
		setLeague(defaultLeague);
		setSort(originalStormSelected
			? { key: 'score', direction: 'descending' }
			: { key: 'rank', direction: 'ascending' });
	}, [availableLeagueIds, boardEntries, currentPlayerId, livePlayerLeague, needsLeagueSelector, originalStormSelected, selectedBoardIdentity]);
	useEffect(() => {
		if (selectedBoard && league !== allLeagues && !availableLeagueIds.includes(Number(league))) {
			setLeague(allLeagues);
		}
	}, [availableLeagueIds, league, selectedBoard]);
	const entries = useMemo(() => {
		const normalizedQuery = searchQuery.trim().toLocaleLowerCase();
		const filtered = boardEntries.filter((entry) => {
			if (league !== allLeagues && entry.leagueId !== Number(league)) return false;
			if (!normalizedQuery) return true;
			const regular = regularPlayers.get(entry.playerId);
			return entry.playerName.toLocaleLowerCase().includes(normalizedQuery)
				|| (entry.allianceName || regular?.allianceName || '').toLocaleLowerCase().includes(normalizedQuery);
		});
		return filtered.sort((left, right) => compareEventRows(left, right, sort, regularPlayers));
	}, [boardEntries, league, regularPlayers, searchQuery, sort]);
	const changeSort = (key: EventSortKey) => {
		setSort((current) => current.key === key
			? { key, direction: current.direction === 'ascending' ? 'descending' : 'ascending' }
			: { key, direction: defaultEventSortDirection(key) });
		setPage(0);
	};
	const pageCount = Math.max(1, Math.ceil(entries.length / eventPageSize));
	const safePage = Math.min(page, pageCount - 1);
	const paginationSelection = eventPaginationSelectionKey({
		worldId,
		eventKey: selectedEventKey,
		runKey: selectedRunKey,
		boardKey: selectedBoardKey,
		league,
		searchQuery,
		sortKey: sort.key,
		sortDirection: sort.direction,
	});
	const previousPaginationSelection = useRef(paginationSelection);
	useEffect(() => {
		const previous = previousPaginationSelection.current;
		previousPaginationSelection.current = paginationSelection;
		setPage((current) => reconcileEventPage(
			current,
			previous,
			paginationSelection,
			entries.length,
			eventPageSize,
		));
	}, [entries.length, paginationSelection]);
	const visibleEntries = entries.slice(safePage * eventPageSize, (safePage + 1) * eventPageSize);
	const loadedBoards = useMemo(() => [
		...Object.values(runBoards).flatMap((candidate) => eventBoardsFromRun(candidate.response)),
		...(stormPublicBoard ? [stormPublicBoard] : []),
	], [runBoards, stormPublicBoard]);
	const loadedScoreRows = loadedBoards.reduce((total, candidate) => total + candidate.entries.length, 0);
	const knownRunCount = Math.max(eventRuns, runs.length);
	const optionalFilterCount = Number(needsRunSelector) + Number(needsBoardSelector) + Number(needsLeagueSelector);
	const filterGridColumns = optionalFilterCount >= 2
		? 'xl:grid-cols-4'
		: optionalFilterCount === 1
			? 'xl:grid-cols-3'
			: 'xl:grid-cols-2';
	const loading = directoryLoading || boardLoading;
	const refreshBoards = useCallback(() => {
		if (selectedRunKey && selectedRunKey !== stormRunKey) {
			// Keep the current board visible while the replacement subscription
			// opens. This preserves both the semantic selection and its page.
			setBoardRefreshToken((current) => current + 1);
		}
		void refreshDirectory();
	}, [refreshDirectory, selectedRunKey]);

	return (
		<div>
			<div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
				<div>
					<div className="flex items-center gap-2 text-base font-bold text-text-main"><Trophy className="h-5 w-5 text-primary" /> Player rankings</div>
						<p className="mt-1 text-xs text-text-muted">Name, Might, Honor, and Alliance stay visible while the selected event adds its Rank and Score columns.</p>
				</div>
				<Button variant="ghost" size="icon" aria-label="Refresh event history" onClick={() => void refreshBoards()} isLoading={loading}><RefreshCw className="h-4 w-4" /></Button>
			</div>
			<div className="mb-4 flex flex-wrap gap-2">
				<Badge variant="outline">{formatCount(knownRunCount)} collected runs</Badge>
				<Badge variant="outline">{eventScores > 0 ? `${formatCount(eventScores)} current score rows` : `${formatCount(loadedScoreRows)} current score rows`}</Badge>
				<Badge variant="outline">{formatCount(loadedBoards.length)} event leaderboards cached</Badge>
				{selectedRunKey && selectedRunKey !== stormRunKey && (
					<Badge variant={boardStreamStatus === 'connected' ? 'success' : 'warning'}>
						{boardLoading || boardStreamStatus === 'connecting' ? 'Loading leaderboard base' : boardStreamStatus === 'connected' ? 'Leaderboard subscribed' : 'Leaderboard fallback active'}
					</Badge>
				)}
			</div>

			{error && <div className="mb-4 rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm text-error" role="alert">{error}</div>}

			{directoryLoading && eventGroups.length === 0 ? (
				<div className="flex min-h-72 items-center justify-center text-sm text-text-muted">Loading event history…</div>
			) : eventGroups.length === 0 ? (
				<EmptyState
					size="md"
					icon={<CalendarDays className="h-6 w-6" />}
					title="No event runs collected yet"
					description="The view is ready for backend 1.3.14 event observations and will populate automatically after a collector uploads this world's first run."
				/>
			) : (
				<>
					<div className={`mb-4 grid gap-3 md:grid-cols-2 ${filterGridColumns}`}>
						<div>
							<div className="mb-1 text-[10px] font-bold uppercase tracking-wider text-text-muted">Event</div>
							<Select
								value={selectedEventKey}
								onChange={(value) => {
									const nextEvent = eventGroups.find((candidate) => candidate.key === value);
									setEvent(value);
									setRun(nextEvent?.publicBoard ? stormRunKey : nextEvent?.runs[0]?.occurrenceId ?? '');
									setBoard('');
									setPage(0);
								}}
								options={eventOptions}
								ariaLabel="Select an event"
								searchable
								disabled={eventOptions.length <= 1}
								menuGrowToViewport
							/>
						</div>
						{needsRunSelector && <div>
							<div className="mb-1 text-[10px] font-bold uppercase tracking-wider text-text-muted">Event dates</div>
							<Select
								value={selectedRunKey}
								onChange={(value) => { setRun(value); setBoard(''); setPage(0); }}
								options={runOptions}
								ariaLabel="Select an event session by date range"
								menuGrowToViewport
							/>
						</div>}
						{needsBoardSelector && <div>
							<div className="mb-1 text-[10px] font-bold uppercase tracking-wider text-text-muted">Leaderboard</div>
							<Select
								value={selectedBoardKey}
								onChange={(value) => { setBoard(value); setPage(0); }}
								options={boardOptions}
								ariaLabel="Select an event leaderboard"
								menuGrowToViewport
							/>
						</div>}
						{needsLeagueSelector && <div>
							<div className="mb-1 text-[10px] font-bold uppercase tracking-wider text-text-muted">Level league</div>
							<Select value={league} onChange={(value) => { setLeague(value); setPage(0); }} options={leagueOptions} ariaLabel="Filter event scores by level league" searchable menuGrowToViewport />
						</div>}
						<div>
							<div className="mb-1 text-[10px] font-bold uppercase tracking-wider text-text-muted">Player or alliance</div>
							<Input
								value={searchQuery}
								onChange={(event) => { setSearchQuery(event.target.value); setPage(0); }}
								placeholder="Search by any part of the name"
								aria-label="Search players or alliances by name"
								leftIcon={<Search className="h-4 w-4" />}
							/>
						</div>
					</div>

					{selectedBoard && (
						<div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
							<MetricTile label="Participants" value={formatCount(selectedBoard.run?.participants ?? selectedBoard.entries.length)} tone="brand" />
							<MetricTile label="Current score rows" value={formatCount(selectedBoard.entries.length)} tone="info" />
							{originalStormSelected
								? <MetricTile label="Ranking scope" value="All levels" monospace={false} />
								: <MetricTile label="Level leagues" value={formatCount(availableLeagueIds.length)} />}
							{selectedBoard.run ? originalStormSelected ? (
								<MetricTile label="Last collected" value={formatDateTime(selectedBoard.run.lastObservedAt)} />
							) : (
								<MetricTile label={eventRunActive(selectedBoard.run) ? 'Ends' : 'Ended'} value={formatDateTime(selectedBoard.run.eventEndsAt)} tone={eventRunActive(selectedBoard.run) ? 'warning' : 'default'} />
							) : (
								<MetricTile label="Updated" value={formatDateTime(latestBoardObservation(selectedBoard.entries))} />
							)}
						</div>
					)}

					<EventScoreTable
						entries={visibleEntries}
						loading={loading}
						regularPlayers={regularPlayers}
						eventTitle={selectedBoard ? eventBoardTitle(selectedBoard) : 'Event'}
						searchQuery={searchQuery}
						sort={sort}
						page={safePage}
						pageCount={pageCount}
						total={entries.length}
						onPageChange={setPage}
						onSort={changeSort}
						onOpenPlayer={onOpenPlayer}
						onOpenAlliance={onOpenAlliance}
					/>
				</>
			)}
		</div>
	);
};

interface PlayerEventHistoryProps {
	history: WorldIntelligencePlayerEventScoreHistoryV1 | null;
	error?: string;
	onOpenAlliance: (allianceId: number, worldId: string) => void;
}

export const WorldPlayerEventHistory = (props: PlayerEventHistoryProps) => (
	<WorldPlayerEventHistoryContent key={props.history?.playerId ?? 'empty'} {...props} />
);

const WorldPlayerEventHistoryContent = ({ history, error = '', onOpenAlliance }: PlayerEventHistoryProps) => {
	const [eventKey, setEventKey] = useState(allEvents);
	const [page, setPage] = useState(0);
	const eventOptions = useMemo(() => {
		const events = new Map<string, string>();
		for (const entry of history?.history ?? []) events.set(entry.eventKey, entry.eventName || humanizeKey(entry.eventKey));
		return [
			{ value: allEvents, label: 'All event history' },
			...[...events].sort((left, right) => left[1].localeCompare(right[1])).map(([value, label]) => ({ value, label })),
		];
	}, [history]);
	const entries = useMemo(() => {
		const matching = (history?.history ?? []).filter((entry) => eventKey === allEvents || entry.eventKey === eventKey);
		return [...matching].reverse();
	}, [eventKey, history]);
	const pageCount = Math.max(1, Math.ceil(entries.length / eventPageSize));
	const safePage = Math.min(page, pageCount - 1);
	const visible = entries.slice(safePage * eventPageSize, (safePage + 1) * eventPageSize);

	return (
		<Card>
			<CardContent>
				<div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
					<div>
						<div className="flex items-center gap-2 font-bold text-text-main"><History className="h-5 w-5 text-primary" /> Event score history</div>
						<p className="mt-1 text-xs text-text-muted">Append-only public rank and score observations for this player, including personal records and rank-only boards.</p>
					</div>
					<div className="w-full sm:w-72">
						<Select value={eventKey} onChange={(value) => { setEventKey(value); setPage(0); }} options={eventOptions} ariaLabel="Filter this player's event history" searchable disabled={eventOptions.length <= 1} menuGrowToViewport />
					</div>
				</div>
				{error && <div className="mb-4 rounded-global border border-warning/30 bg-warning/10 px-4 py-3 text-sm text-warning" role="status">{error}</div>}
				{entries.length === 0 ? (
					<EmptyState size="sm" surface="plain" icon={<Trophy className="h-5 w-5" />} title="No event scores observed yet" description="This section will populate as event boards are collected for this player." />
				) : (
					<PlayerEventScoreTable entries={visible} page={safePage} pageCount={pageCount} total={entries.length} onPageChange={setPage} onOpenAlliance={onOpenAlliance} />
				)}
			</CardContent>
		</Card>
	);
};

const EventScoreTable = ({ entries, loading, regularPlayers, eventTitle, searchQuery, sort, page, pageCount, total, onPageChange, onSort, onOpenPlayer, onOpenAlliance }: {
	entries: EventLeaderboardRow[];
	loading: boolean;
	regularPlayers: Map<number, WorldIntelligenceRankingEntryV1>;
	eventTitle: string;
	searchQuery: string;
	sort: EventSort;
	page: number;
	pageCount: number;
	total: number;
	onPageChange: (page: number) => void;
	onSort: (key: EventSortKey) => void;
	onOpenPlayer: (playerId: number, worldId: string) => void;
	onOpenAlliance: (allianceId: number, worldId: string) => void;
}) => {
	if (loading && entries.length === 0) return <div className="flex min-h-72 items-center justify-center text-sm text-text-muted">Loading event leaderboard…</div>;
	return (
		<div className={`overflow-hidden rounded-global border border-border-base transition-opacity ${loading ? 'opacity-60' : ''}`} aria-busy={loading}>
			<div className="max-h-[42rem] overflow-auto custom-scrollbar">
				<table className="min-w-[68rem] w-full text-sm">
					<thead className="sticky top-0 z-10 bg-bg-card text-[10px] uppercase tracking-wide text-text-muted">
						<tr>
							<SortableEventHeader rowSpan={2} label="Name" column="name" sort={sort} onSort={onSort} className="min-w-56 text-left" />
							<SortableEventHeader rowSpan={2} label="Might" column="might" sort={sort} onSort={onSort} className="min-w-32 text-right" align="right" />
							<SortableEventHeader rowSpan={2} label="Honor" column="honor" sort={sort} onSort={onSort} className="min-w-32 text-right" align="right" />
							<SortableEventHeader rowSpan={2} label="Alliance" column="alliance" sort={sort} onSort={onSort} className="min-w-52 text-left" />
							<th colSpan={2} className="border-l border-border-base px-3 py-2 text-center text-primary">{eventTitle}</th>
						</tr>
						<tr className="border-t border-border-base"><SortableEventHeader label="Rank" column="rank" sort={sort} onSort={onSort} className="border-l border-border-base text-right" align="right" /><SortableEventHeader label="Score" column="score" sort={sort} onSort={onSort} className="text-right" align="right" /></tr>
					</thead>
					<tbody>
						{entries.length === 0 ? (
							<tr className="border-t border-border-base"><td colSpan={6} className="px-4 py-14 text-center"><div className="font-bold text-text-main">{searchQuery.trim() ? `No player or alliance contains “${searchQuery.trim()}”` : 'No scores for this event yet'}</div><div className="mt-1 text-xs text-text-muted">{searchQuery.trim() ? 'Try a shorter part of the player or alliance name.' : 'Try another event. Empty and rank-only observations are never converted into invented scores.'}</div></td></tr>
						) : entries.map((entry) => {
							const regular = regularPlayers.get(entry.playerId);
							const allianceId = entry.allianceId ?? regular?.allianceId;
							const allianceName = entry.allianceName || regular?.allianceName;
							return <tr key={`${eventBoardIdentity(entry)}:${entry.leagueId}:${entry.playerId}`} className="border-t border-border-base hover:bg-bg-card-hover">
								<td className="px-3 py-2.5"><button type="button" className="max-w-64 truncate text-left font-bold text-text-main hover:text-primary" onClick={() => onOpenPlayer(entry.playerId, entry.worldId)}>{entry.playerName}</button></td>
								<RegularMetricValue value={regular?.might} />
								<RegularMetricValue value={regular?.honor} />
								<td className="px-3 py-2.5">{allianceId ? <button type="button" className="max-w-56 truncate font-semibold text-text-main hover:text-primary" onClick={() => onOpenAlliance(allianceId, entry.worldId)}>{allianceName || `Alliance ${allianceId}`}</button> : <span className="text-text-muted">No alliance</span>}</td>
								<td className="border-l border-border-base px-3 py-2.5 text-right font-mono font-black text-primary">#{formatCount(entry.rank)}</td>
								<td className="px-3 py-2.5 text-right"><EventScoreValue entry={entry} /></td>
							</tr>;
						})}
					</tbody>
				</table>
			</div>
			<TablePager page={page} pageCount={pageCount} total={total} noun="scores" onPageChange={onPageChange} />
		</div>
	);
};

const SortableEventHeader = ({ label, column, sort, onSort, rowSpan, className = '', align = 'left' }: {
	label: string;
	column: EventSortKey;
	sort: EventSort;
	onSort: (column: EventSortKey) => void;
	rowSpan?: number;
	className?: string;
	align?: 'left' | 'right';
}) => {
	const active = sort.key === column;
	const Icon = !active ? ArrowUpDown : sort.direction === 'ascending' ? ArrowUp : ArrowDown;
	return (
		<th rowSpan={rowSpan} aria-sort={active ? sort.direction : 'none'} className={`px-3 py-2 align-middle ${className}`}>
			<button type="button" className={`inline-flex w-full items-center gap-1.5 font-bold transition-colors hover:text-primary ${align === 'right' ? 'justify-end' : 'justify-start'} ${active ? 'text-primary' : ''}`} onClick={() => onSort(column)}>
				{label}<Icon className={`h-3.5 w-3.5 ${active ? 'opacity-100' : 'opacity-45'}`} />
			</button>
		</th>
	);
};

const PlayerEventScoreTable = ({ entries, page, pageCount, total, onPageChange, onOpenAlliance }: {
	entries: WorldIntelligenceEventScoreObservationV1[];
	page: number;
	pageCount: number;
	total: number;
	onPageChange: (page: number) => void;
	onOpenAlliance: (allianceId: number, worldId: string) => void;
}) => (
	<div className="overflow-hidden rounded-global border border-border-base">
		<div className="max-h-[36rem] overflow-auto custom-scrollbar">
			<table className="min-w-[70rem] w-full text-sm">
				<thead className="sticky top-0 z-10 bg-bg-card text-[10px] uppercase tracking-wide text-text-muted">
					<tr><th className="px-3 py-2 text-left">Observed</th><th className="px-3 py-2 text-left">Event run</th><th className="px-3 py-2 text-left">Board</th><th className="px-3 py-2 text-right">Rank</th><th className="px-3 py-2 text-right">Score</th><th className="px-3 py-2 text-left">Alliance</th></tr>
				</thead>
				<tbody>
					{entries.map((entry, index) => (
						<tr key={`${entry.occurrenceId}:${entry.listType}:${entry.leagueId}:${entry.observedAt}:${index}`} className="border-t border-border-base hover:bg-bg-card-hover">
							<td className="whitespace-nowrap px-3 py-2.5 text-xs text-text-muted">{formatDateTime(entry.observedAt)}</td>
								<td className="px-3 py-2.5"><div className="font-bold text-text-main">{entry.eventName || humanizeKey(entry.eventKey)}</div><div className="text-[11px] text-text-muted">{isOriginalStormRanking(entry.eventId, entry.listType) ? `Monthly session · ${formatDate(entry.runStartedOn)}` : `Started ${formatDate(entry.runStartedOn)} · ends ${formatDateTime(entry.eventEndsAt)}`}</div></td>
								<td className="px-3 py-2.5 text-xs text-text-muted" title={isOriginalStormRanking(entry.eventId, entry.listType) ? `Event ${entry.eventId} · list ${entry.listType} · all levels` : `Event ${entry.eventId} · list ${entry.listType} · league ${entry.leagueId}`}>{eventBoardLabel(entry)}</td>
							<td className="px-3 py-2.5 text-right font-mono font-black text-primary">#{formatCount(entry.rank)}</td>
							<td className="px-3 py-2.5 text-right"><EventScoreValue entry={entry} /></td>
							<td className="px-3 py-2.5">{entry.allianceId ? <button type="button" className="max-w-56 truncate font-semibold text-text-main hover:text-primary" onClick={() => onOpenAlliance(entry.allianceId!, entry.worldId)}>{entry.allianceName || `Alliance ${entry.allianceId}`}</button> : <span className="text-text-muted">No alliance</span>}</td>
						</tr>
					))}
				</tbody>
			</table>
		</div>
		<TablePager page={page} pageCount={pageCount} total={total} noun="observations" onPageChange={onPageChange} />
	</div>
);

const EventScoreValue = ({ entry }: { entry: Pick<EventLeaderboardRow, 'score' | 'scoreKnown' | 'scoreUnit'> }) => entry.scoreKnown ? (
	<div><div className="font-mono font-black text-text-main">{formatCount(entry.score)}</div><div className="text-[10px] uppercase tracking-wide text-text-muted">{entry.scoreUnit || 'points'}</div></div>
) : <Badge variant="warning">Rank only</Badge>;

const RegularMetricValue = ({ value }: { value?: number }) => (
	<td className="px-3 py-2.5 text-right font-mono font-semibold text-text-main">
		{value == null ? <span className="text-text-muted">—</span> : formatCount(value)}
	</td>
);

const TablePager = ({ page, pageCount, total, noun, onPageChange }: { page: number; pageCount: number; total: number; noun: string; onPageChange: (page: number) => void }) => {
	const first = total === 0 ? 0 : page * eventPageSize + 1;
	const last = Math.min(total, first + eventPageSize - 1);
	return (
		<div className="flex flex-col gap-2 border-t border-border-base bg-bg-input/25 px-4 py-3 text-xs text-text-muted sm:flex-row sm:items-center sm:justify-between">
			<span>{formatCount(first)}–{formatCount(last)} of {formatCount(total)} {noun}</span>
			<div className="flex items-center gap-2">
				<Button type="button" variant="ghost" size="sm" disabled={page <= 0} onClick={() => onPageChange(page - 1)}><ChevronLeft className="mr-1 h-4 w-4" />Previous</Button>
				<span>Page {page + 1} of {pageCount}</span>
				<Button type="button" variant="ghost" size="sm" disabled={page + 1 >= pageCount} onClick={() => onPageChange(page + 1)}>Next<ChevronRight className="ml-1 h-4 w-4" /></Button>
			</div>
		</div>
	);
};

function defaultEventSortDirection(key: EventSortKey): EventSort['direction'] {
	return key === 'name' || key === 'alliance' || key === 'rank' ? 'ascending' : 'descending';
}

function compareEventRows(
	left: EventLeaderboardRow,
	right: EventLeaderboardRow,
	sort: EventSort,
	regularPlayers: Map<number, WorldIntelligenceRankingEntryV1>,
): number {
	const leftValue = eventSortValue(left, sort.key, regularPlayers);
	const rightValue = eventSortValue(right, sort.key, regularPlayers);
	if (leftValue == null && rightValue == null) return left.rank - right.rank;
	if (leftValue == null) return 1;
	if (rightValue == null) return -1;
	const comparison = typeof leftValue === 'string' && typeof rightValue === 'string'
		? leftValue.localeCompare(rightValue, undefined, { sensitivity: 'base' })
		: Number(leftValue) - Number(rightValue);
	return (sort.direction === 'ascending' ? comparison : -comparison) || left.rank - right.rank;
}

function eventSortValue(
	entry: EventLeaderboardRow,
	key: EventSortKey,
	regularPlayers: Map<number, WorldIntelligenceRankingEntryV1>,
): string | number | null {
	const regular = regularPlayers.get(entry.playerId);
	switch (key) {
	case 'name': return entry.playerName;
	case 'might': return finiteEventValue(regular?.might);
	case 'honor': return finiteEventValue(regular?.honor);
	case 'alliance': return entry.allianceName || regular?.allianceName || null;
	case 'rank': return entry.rank;
	case 'score': return entry.scoreKnown ? finiteEventValue(entry.score) : null;
	}
}

function finiteEventValue(value: number | undefined): number | null {
	return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

function eventBoardIdentity(entry: Pick<EventLeaderboardRow, 'listType' | 'boardKey'>): string {
	return `${entry.listType}:${entry.boardKey ?? ''}`;
}

function eventBoardTitle(board: EventBoard): string {
	const publicVariant = board.boardKey ? ` — ${humanizeKey(board.boardKey)}` : '';
	return `${board.eventName}${publicVariant}`;
}

function orderedEventRuns(allRuns: readonly WorldIntelligenceEventRunV1[]): WorldIntelligenceEventRunV1[] {
	return [...allRuns].sort((left, right) => Date.parse(right.eventEndsAt) - Date.parse(left.eventEndsAt));
}

function eventBoardsFromRun(response: WorldIntelligenceEventRunRankingV1): EventBoard[] {
	const groups = new Map<string, WorldIntelligenceEventScoreObservationV1[]>();
	for (const entry of normalizeStormSessionRanking(response.run.eventId, response.entries ?? [])) {
		const identity = eventBoardIdentity(entry);
		const entries = groups.get(identity) ?? [];
		entries.push(entry);
		groups.set(identity, entries);
	}
	const boards: EventBoard[] = [];
	for (const [identity, entries] of groups) {
		const sample = entries[0];
		boards.push({
			key: `${response.run.occurrenceId}:${identity}`,
			eventId: response.run.eventId,
			eventName: response.run.eventName || humanizeKey(response.run.eventKey),
			listType: sample.listType,
			boardKey: sample.boardKey ?? '',
			entries,
			run: response.run,
		});
	}
	if (boards.length === 0) {
		boards.push({
			key: `${response.run.occurrenceId}:empty`,
			eventId: response.run.eventId,
			eventName: response.run.eventName || humanizeKey(response.run.eventKey),
			listType: 0,
			boardKey: '',
			entries: [],
			run: response.run,
		});
	}
	return boards.sort((left, right) => left.listType - right.listType || left.boardKey.localeCompare(right.boardKey));
}

function applyEventRunRankingDelta(
	current: WorldIntelligenceEventRunRankingV1,
	delta: WorldIntelligenceEventRunRankingDeltaV1,
): WorldIntelligenceEventRunRankingV1 {
	const entries = new Map<string, WorldIntelligenceEventScoreObservationV1>();
	for (const entry of current.entries ?? []) entries.set(eventScoreIdentity(entry), entry);
	for (const removed of delta.removed ?? []) entries.delete(eventScoreIdentity(removed));
	for (const entry of delta.upserts ?? []) entries.set(eventScoreIdentity(entry), entry);
	return {
		run: delta.run,
		entries: [...entries.values()].sort((left, right) => (
			left.listType - right.listType
			|| left.leagueId - right.leagueId
			|| left.rank - right.rank
			|| left.playerId - right.playerId
		)),
	};
}

function eventScoreIdentity(
	entry: Pick<WorldIntelligenceEventScoreObservationV1, 'listType' | 'leagueId' | 'playerId'>,
): string {
	return `${entry.listType}:${entry.leagueId}:${entry.playerId}`;
}

function mergeRankingPlayers(
	snapshots: ReferenceRankingSnapshots,
): Map<number, WorldIntelligenceRankingEntryV1> {
	const next = new Map<number, WorldIntelligenceRankingEntryV1>();
	for (const key of ['might', 'honor', 'storm'] as const) {
		const response = snapshots[key];
		if (response == null) continue;
		for (const entry of response.entries ?? []) {
			const existing = next.get(entry.id);
			next.set(entry.id, {
				...existing,
				...entry,
				might: entry.might ?? existing?.might,
				honor: entry.honor ?? existing?.honor,
			});
		}
	}
	return next;
}

function stormPublicRankingBoard(response: WorldIntelligenceRankingResponseV1 | null): EventBoard | null {
	const entries = response?.entries ?? [];
	if (entries.length === 0) return null;
	return {
		key: stormRunKey,
		eventId: stormBoard.eventId,
		eventName: stormBoard.eventName,
		listType: stormBoard.listType,
		boardKey: '',
		entries: entries.map((entry) => ({
			worldId: entry.worldId,
			eventId: stormBoard.eventId,
			listType: stormBoard.listType,
			leagueId: stormBoard.leagueId,
			playerId: entry.id,
			playerName: entry.name,
			allianceId: entry.allianceId,
			allianceName: entry.allianceName,
			rank: entry.rank,
			score: entry.value,
			scoreKnown: true,
			scoreUnit: 'points',
			observedAt: entry.lastObservedAt,
		})),
	};
}

function groupEventRuns(runs: readonly WorldIntelligenceEventRunV1[], publicBoard: EventBoard | null): EventRunGroup[] {
	const groups = new Map<string, EventRunGroup>();
	for (const run of runs) {
		const eventKey = run.eventKey.trim().toLocaleLowerCase() || `event-${run.eventId}`;
		const key = `${eventKey}:${run.eventId}`;
		const group = groups.get(key) ?? {
			key,
			title: run.eventName || humanizeKey(run.eventKey),
			runs: [],
		};
		group.runs.push(run);
		groups.set(key, group);
	}
	if (publicBoard) {
		const stormEventKey = `storm-islands:${stormBoard.eventId}`;
		const stormGroup = groups.get(stormEventKey) ?? {
			key: stormEventKey,
			title: publicBoard.eventName,
			runs: [],
		};
		// Current Storm inclusion is defined by cargo metrics, not optional
		// occurrence metadata. Keep the live metric board available alongside
		// dated history even when only some collectors could anchor a session.
		stormGroup.publicBoard = publicBoard;
		groups.set(stormEventKey, stormGroup);
	}
	return [...groups.values()]
		.map((group) => ({
			...group,
			runs: group.runs.sort((left, right) => Date.parse(right.eventEndsAt) - Date.parse(left.eventEndsAt)),
		}))
		.sort((left, right) => left.title.localeCompare(right.title) || left.key.localeCompare(right.key));
}

function eventRunLabel(run: WorldIntelligenceEventRunV1): string {
	const startTimestamp = Date.parse(`${run.runStartedOn}T00:00:00Z`);
	const endTimestamp = Date.parse(run.eventEndsAt);
	if (isOriginalStormRanking(run.eventId, stormBoard.listType) && Number.isFinite(startTimestamp)) {
		return `${new Intl.DateTimeFormat(undefined, { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(startTimestamp))} session`;
	}
	if (Number.isFinite(startTimestamp) && Number.isFinite(endTimestamp)) {
		return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeZone: 'UTC' })
			.formatRange(new Date(startTimestamp), new Date(endTimestamp));
	}
	const start = formatDate(run.runStartedOn);
	const end = formatDate(run.eventEndsAt);
	return start === end ? end : `${start} – ${end}`;
}

function eventBoardVariantLabel(board: EventBoard): string {
	if (board.boardKey) return `${humanizeKey(board.boardKey)} · List ${board.listType}`;
	return board.listType > 0 ? `List ${board.listType}` : 'Leaderboard';
}

function eventBoardLabel(entry: Pick<EventLeaderboardRow, 'eventId' | 'listType' | 'boardKey' | 'leagueId'>): string {
	if (isOriginalStormRanking(entry.eventId, entry.listType)) return 'Storm ranking · All levels';
	const parts = [entry.boardKey ? humanizeKey(entry.boardKey) : `List ${entry.listType}`];
	if (entry.boardKey) parts.push(`List ${entry.listType}`);
	parts.push(entry.leagueId >= 0 ? `League ${entry.leagueId}` : 'No league');
	return parts.join(' · ');
}

function latestBoardObservation(entries: EventLeaderboardRow[]): string {
	let latest = '';
	let latestTimestamp = Number.NEGATIVE_INFINITY;
	for (const entry of entries) {
		const timestamp = Date.parse(entry.observedAt);
		if (Number.isFinite(timestamp) && timestamp > latestTimestamp) {
			latest = entry.observedAt;
			latestTimestamp = timestamp;
		}
	}
	return latest;
}

function parseLeagueDefinitions(rows: Array<Record<string, unknown>>): LeagueDefinition[] {
	const definitions: LeagueDefinition[] = [];
	for (const row of rows) {
		const eventId = integerValue(row.eventID);
		const leagueId = integerValue(row.leaguetypeID);
		const minimumLevel = integerValue(row.minLevel);
		const maximumLevel = integerValue(row.maxLevel);
		const subType = integerValue(row.subType);
		if (eventId == null || leagueId == null || minimumLevel == null || maximumLevel == null || subType != null) continue;
		definitions.push({ eventId, leagueId, minimumLevel, maximumLevel });
	}
	return definitions;
}

function levelLeagueLabel(eventId: number | undefined, leagueId: number, definitions: LeagueDefinition[]): string {
	if (leagueId < 0) return 'No level league';
	const definition = definitions.find((candidate) => candidate.eventId === eventId && candidate.leagueId === leagueId)
		?? definitions.find((candidate) => candidate.eventId === -1 && candidate.leagueId === leagueId);
	if (!definition) return `League ${leagueId}`;
	return `League ${leagueId} · ${playerLevelRange(definition.minimumLevel, definition.maximumLevel)}`;
}

function playerLevelRange(minimum: number, maximum: number): string {
	if (maximum < 70) return minimum === maximum ? `Level ${minimum}` : `Levels ${minimum}–${maximum}`;
	if (minimum < 70) return `Levels ${minimum}–69 · Legendary 0–${Math.max(0, maximum - 70)}`;
	const minimumLegend = Math.max(0, minimum - 70);
	const maximumLegend = Math.max(0, maximum - 70);
	return minimumLegend === maximumLegend
		? `Level 70 · Legendary ${minimumLegend}`
		: `Level 70 · Legendary ${minimumLegend}–${maximumLegend}`;
}

function integerValue(value: unknown): number | null {
	if (value == null || value === '') return null;
	const parsed = typeof value === 'number' ? value : Number(value);
	return Number.isInteger(parsed) ? parsed : null;
}

function eventRunActive(run: WorldIntelligenceEventRunV1): boolean {
	return Number.isFinite(Date.parse(run.eventEndsAt)) && Date.parse(run.eventEndsAt) > Date.now();
}

function humanizeKey(value: string): string {
	return value.replace(/[_-]+/g, ' ').replace(/([a-z0-9])([A-Z])/g, '$1 $2').replace(/^./, (character) => character.toUpperCase());
}

function normalizeWorldID(value: string): string {
	const trimmed = value.trim();
	if (!trimmed) return '';
	try {
		const parsed = new URL(trimmed.includes('://') ? trimmed : `https://${trimmed}`);
		const port = parsed.port && parsed.port !== '80' && parsed.port !== '443' ? `:${parsed.port}` : '';
		return `${parsed.hostname.toLocaleLowerCase()}${port}`;
	} catch {
		return trimmed.toLocaleLowerCase().replace(/^wss?:\/\//, '').split('/')[0].replace(/:(80|443)$/, '');
	}
}

function observationAtLeast(current: string, candidate: string): boolean {
	const currentTimestamp = Date.parse(current);
	const candidateTimestamp = Date.parse(candidate);
	if (Number.isFinite(currentTimestamp) && Number.isFinite(candidateTimestamp)) {
		return currentTimestamp >= candidateTimestamp;
	}
	return current !== '' && current === candidate;
}

function formatCount(value?: number): string {
	return new Intl.NumberFormat().format(value ?? 0);
}

function formatDate(value: string): string {
	const timestamp = Date.parse(/^\d{4}-\d{2}-\d{2}$/.test(value) ? `${value}T00:00:00Z` : value);
	return Number.isFinite(timestamp) ? new Date(timestamp).toLocaleDateString(undefined, { dateStyle: 'medium', timeZone: 'UTC' }) : value || 'Unknown';
}

function formatDateTime(value: string): string {
	const timestamp = Date.parse(value);
	return Number.isFinite(timestamp) ? new Date(timestamp).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : 'Unknown';
}

function errorMessage(error: unknown, fallback: string): string {
	return error instanceof Error && error.message ? error.message : fallback;
}
