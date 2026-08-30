import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
	CloudOff,
	Database,
	UserRound,
	Users,
	X,
} from 'lucide-react';
import { CitadelAPI, type WorldIntelligenceSubscriptionStatus } from '../../api/CitadelClient';
import type {
	WorldIntelligenceAllianceProfileV1,
	WorldIntelligenceCoverageResponseV1,
	WorldIntelligencePlayerEventScoreHistoryV1,
	WorldIntelligencePlayerProfileV1,
	WorldIntelligenceStatusV1,
	WorldIntelligenceUpdateManifestV1,
} from '../../api/Contracts';
import {
	Badge,
	Card,
	CardContent,
	EmptyState,
	PageHeader,
	SectionCard,
} from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import DetailBackButton from '../../components/DetailBackButton';
import WorldPlayerDetailView from './WorldPlayerDetailView';
import WorldAllianceDetailView from './WorldAllianceDetailView';
import { WorldEventHistory, WorldPlayerEventHistory } from './WorldEventHistory';

type SelectedEntity = { type: 'player' | 'alliance'; id: number; worldId: string };

const WorldIntelligenceView = () => {
	const { state } = useCitadelAPI();
	const [status, setStatus] = useState<WorldIntelligenceStatusV1 | null>(null);
	const worldId = state?.account.worldId || status?.worldId || state?.session.serverUrl || '';
	const [coverage, setCoverage] = useState<WorldIntelligenceCoverageResponseV1>({ worlds: [] });
	const [coverageError, setCoverageError] = useState('');
	const [worldUpdate, setWorldUpdate] = useState<WorldIntelligenceUpdateManifestV1 | null>(null);
	const [subscriptionStatus, setSubscriptionStatus] = useState<WorldIntelligenceSubscriptionStatus>('connecting');
	const [selected, setSelected] = useState<SelectedEntity | null>(null);
	const [playerProfile, setPlayerProfile] = useState<WorldIntelligencePlayerProfileV1 | null>(null);
	const [playerEventHistory, setPlayerEventHistory] = useState<WorldIntelligencePlayerEventScoreHistoryV1 | null>(null);
	const [playerEventError, setPlayerEventError] = useState('');
	const [allianceProfile, setAllianceProfile] = useState<WorldIntelligenceAllianceProfileV1 | null>(null);
	const [profileLoading, setProfileLoading] = useState(false);
	const [error, setError] = useState('');
	const directoryScrollRef = useRef(0);
	const previousWorldUpdate = useRef<WorldIntelligenceUpdateManifestV1 | null>(null);

	const refreshStatus = useCallback(async () => {
		try {
			setStatus(await CitadelAPI.getWorldIntelligenceStatus());
		} catch (requestError) {
			setError(errorMessage(requestError, 'Could not read World Intelligence status.'));
		}
	}, []);

	const refreshCoverage = useCallback(async () => {
		if (!worldId) {
			setCoverage({ worlds: [] });
			setCoverageError('');
			return;
		}
		try {
			setCoverage(await CitadelAPI.getWorldIntelligenceCoverage(worldId));
			setCoverageError('');
		} catch (requestError) {
			setCoverage({ worlds: [] });
			setCoverageError(errorMessage(requestError, 'Cloud coverage is temporarily unavailable.'));
		}
	}, [worldId]);

	useEffect(() => {
		void refreshStatus();
		const interval = window.setInterval(() => void refreshStatus(), 30_000);
		return () => window.clearInterval(interval);
	}, [refreshStatus]);

	useEffect(() => {
		const timeout = window.setTimeout(() => void refreshCoverage(), 10_000);
		return () => window.clearTimeout(timeout);
	}, [refreshCoverage]);

	useEffect(() => {
		if (!worldId || subscriptionStatus === 'connected') return;
		const interval = window.setInterval(() => void refreshCoverage(), 30_000);
		return () => window.clearInterval(interval);
	}, [refreshCoverage, subscriptionStatus, worldId]);

	useEffect(() => {
		previousWorldUpdate.current = null;
		setWorldUpdate(null);
		if (!worldId) {
			setSubscriptionStatus('connecting');
			return;
		}
		return CitadelAPI.subscribeWorldIntelligenceUpdates(worldId, setWorldUpdate, setSubscriptionStatus);
	}, [worldId]);

	useEffect(() => {
		if (!worldUpdate || worldUpdate.worldId !== displayWorld(worldId)) return;
		const previous = previousWorldUpdate.current;
		previousWorldUpdate.current = worldUpdate;
		if (previous && worldUpdate.coverageRevision > previous.coverageRevision) {
			void refreshCoverage();
		}
	}, [refreshCoverage, worldId, worldUpdate]);

	const openEntity = useCallback(async (entity: SelectedEntity) => {
		if (!selected) directoryScrollRef.current = window.scrollY;
		setSelected(entity);
		setPlayerProfile(null);
		setPlayerEventHistory(null);
		setPlayerEventError('');
		setAllianceProfile(null);
		setProfileLoading(true);
		setError('');
		window.scrollTo({ top: 0, behavior: 'smooth' });
		try {
			if (entity.type === 'player') {
				const [profileResult, eventHistoryResult] = await Promise.allSettled([
					CitadelAPI.getWorldIntelligencePlayer(entity.worldId, entity.id, 2_000),
					CitadelAPI.getWorldIntelligencePlayerEventScores({ worldId: entity.worldId, playerId: entity.id, limit: 5_000 }),
				]);
				if (profileResult.status === 'rejected') throw profileResult.reason;
				setPlayerProfile(profileResult.value);
				if (eventHistoryResult.status === 'fulfilled') {
					setPlayerEventHistory(eventHistoryResult.value);
				} else {
					setPlayerEventError(errorMessage(eventHistoryResult.reason, 'Event score history is temporarily unavailable.'));
				}
			} else {
				setAllianceProfile(await CitadelAPI.getWorldIntelligenceAlliance(entity.worldId, entity.id));
			}
		} catch (requestError) {
			setError(errorMessage(requestError, 'Could not load this profile.'));
		} finally {
			setProfileLoading(false);
		}
	}, [selected]);

	const closeProfile = () => {
		const directoryScroll = directoryScrollRef.current;
		setSelected(null);
		setPlayerProfile(null);
		setPlayerEventHistory(null);
		setPlayerEventError('');
		setAllianceProfile(null);
		setError('');
		window.requestAnimationFrame(() => window.scrollTo({ top: directoryScroll }));
	};

	const currentCoverage = coverage.worlds[0];
	const currentLeagueByEvent = useMemo(() => {
		const leagues: Record<number, number> = {};
		for (const event of Object.values(state?.eventScores.byEvent ?? {})) {
			if (event.eventId > 0 && event.leagueId != null) leagues[event.eventId] = event.leagueId;
		}
		return leagues;
	}, [state?.eventScores.byEvent]);
	const featureReady = Boolean(worldId);

	if (selected) {
		return (
			<div className="flex flex-col gap-6 pb-8">
				<nav aria-label="World Intelligence detail navigation" className="sticky top-3 z-30 self-start">
					<DetailBackButton label="Back to World Intelligence" onClick={closeProfile} className="shadow-lg backdrop-blur" />
				</nav>
				{error && (
					<div className="flex items-start justify-between gap-3 rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm text-error" role="alert">
						<span>{error}</span>
						<button type="button" aria-label="Dismiss error" onClick={() => setError('')}><X className="h-4 w-4" /></button>
					</div>
				)}
				{profileLoading ? (
					<>
						<PageHeader
							eyebrow="World Intelligence dossier"
							title={selected.type === 'player' ? 'Loading player…' : 'Loading alliance…'}
							description={`Loading public history from ${displayWorld(selected.worldId)}`}
							icon={selected.type === 'player' ? <UserRound className="h-6 w-6" /> : <Users className="h-6 w-6" />}
						/>
						<Card><CardContent className="flex min-h-72 items-center justify-center text-sm text-text-muted">Loading public history…</CardContent></Card>
					</>
					) : playerProfile ? (
						<>
							<WorldPlayerDetailView
								profile={playerProfile}
								onOpenAlliance={(allianceId) => void openEntity({ type: 'alliance', id: allianceId, worldId: playerProfile.current.worldId })}
							/>
							<WorldPlayerEventHistory
								history={playerEventHistory}
								error={playerEventError}
								onOpenAlliance={(allianceId, entityWorldId) => void openEntity({ type: 'alliance', id: allianceId, worldId: entityWorldId })}
							/>
						</>
				) : allianceProfile ? (
					<WorldAllianceDetailView profile={allianceProfile} onOpenPlayer={(player) => void openEntity({ type: 'player', id: player.playerId, worldId: player.worldId })} />
				) : (
					<>
						<PageHeader
							eyebrow="World Intelligence dossier"
							title="Profile unavailable"
							description={`No public profile was returned for ${displayWorld(selected.worldId)}`}
							icon={selected.type === 'player' ? <UserRound className="h-6 w-6" /> : <Users className="h-6 w-6" />}
						/>
						<EmptyState size="lg" title="Profile unavailable" description="No usable public observations were returned for this entity." />
					</>
				)}
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-6 pb-8">
			{error && (
				<div className="flex items-start justify-between gap-3 rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm text-error" role="alert">
					<span>{error}</span>
					<button type="button" aria-label="Dismiss error" onClick={() => setError('')}><X className="h-4 w-4" /></button>
				</div>
			)}

			{(featureReady || currentCoverage || coverageError) && <div className="flex flex-wrap items-center gap-2">
				{featureReady && <Badge variant={subscriptionStatus === 'connected' ? 'success' : 'warning'}>
					{subscriptionStatus === 'connected' ? 'Live update stream' : subscriptionStatus === 'connecting' ? 'Connecting update stream' : 'Update stream reconnecting'}
				</Badge>}
				{currentCoverage ? (
					<>
						<Badge variant="outline">{formatCount(currentCoverage.players)} players</Badge>
						<Badge variant="outline">{formatCount(currentCoverage.alliances)} alliances</Badge>
						<Badge variant="outline">{formatCount(currentCoverage.holdings)} holdings</Badge>
						<Badge variant="outline">{formatCount(currentCoverage.eventRuns)} event runs</Badge>
						<Badge variant="outline">{formatCount(currentCoverage.eventScores)} event scores</Badge>
						<Badge variant="outline">{formatCount(currentCoverage.observationCount)} current records</Badge>
						<Badge variant={freshnessTone(currentCoverage.lastObservedAt)}>Updated {currentCoverage.lastObservedAt ? relativeTime(currentCoverage.lastObservedAt) : 'never'}</Badge>
					</>
				) : coverageError ? (
					<Badge variant="warning" title={coverageError}>Coverage totals unavailable</Badge>
				) : null}
			</div>}

			{!featureReady ? (
				<EmptyState
					size="lg"
					icon={<CloudOff className="h-7 w-7" />}
					title="Connect a game world first"
					description="The active game world is required so players with the same ID on different servers never get mixed."
				/>
			) : (
				<SectionCard
					title="World rankings"
					description={`One event-aware player table for ${displayWorld(worldId)} with permanent identity, Might, Honor, and Alliance columns.`}
					icon={<Database className="h-5 w-5" />}
				>
					<WorldEventHistory
						worldId={worldId}
						eventRuns={currentCoverage?.eventRuns}
						eventScores={currentCoverage?.eventScores}
						currentPlayerId={state?.player.id}
						currentLeagueByEvent={currentLeagueByEvent}
						worldUpdate={worldUpdate}
						updateStreamConnected={subscriptionStatus === 'connected'}
						onOpenPlayer={(playerId, entityWorldId) => void openEntity({ type: 'player', id: playerId, worldId: entityWorldId })}
						onOpenAlliance={(allianceId, entityWorldId) => void openEntity({ type: 'alliance', id: allianceId, worldId: entityWorldId })}
					/>
				</SectionCard>
			)}
		</div>
	);
};

function displayWorld(value: string): string {
	const trimmed = value.trim();
	if (!trimmed) return '';
	try {
		const parsed = new URL(trimmed.includes('://') ? trimmed : `https://${trimmed}`);
		const port = parsed.port && parsed.port !== '443' && parsed.port !== '80' ? `:${parsed.port}` : '';
		return `${parsed.hostname}${port}` || trimmed;
	} catch {
		return trimmed.replace(/^wss?:\/\//, '').split('/')[0].replace(/:(443|80)$/, '');
	}
}

function formatCount(value?: number): string {
	return new Intl.NumberFormat().format(value ?? 0);
}

function relativeTime(value: string): string {
	const timestamp = Date.parse(value);
	if (!Number.isFinite(timestamp)) return 'Unknown';
	const delta = Math.round((Date.now() - timestamp) / 1000);
	const seconds = Math.abs(delta);
	if (seconds < 60) return 'just now';
	if (delta < 0 && seconds < 3600) return `in ${Math.ceil(seconds / 60)}m`;
	if (delta < 0 && seconds < 86_400) return `in ${Math.ceil(seconds / 3600)}h`;
	if (delta < 0) return `in ${Math.ceil(seconds / 86_400)}d`;
	if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
	if (seconds < 86_400) return `${Math.floor(seconds / 3600)}h ago`;
	return `${Math.floor(seconds / 86_400)}d ago`;
}

function freshnessTone(value?: string): 'outline' | 'success' | 'warning' {
	if (!value) return 'outline';
	const age = Date.now() - Date.parse(value);
	return age <= 60 * 60 * 1000 ? 'success' : 'warning';
}

function errorMessage(error: unknown, fallback: string): string {
	return error instanceof Error && error.message ? error.message : fallback;
}

export default WorldIntelligenceView;
