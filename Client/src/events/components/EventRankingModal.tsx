import React, { useMemo } from 'react';
import { RefreshCw, Trophy } from 'lucide-react';
import type { EventRankingStateV2 } from '../../api/Contracts';
import { Badge, Button, EmptyState, MetricTile, Modal } from '../../components/ui';

interface EventRankingModalProps {
	isOpen: boolean;
	eventName: string;
	ranking?: EventRankingStateV2;
	allianceId?: number;
	isRefreshing: boolean;
	error?: string;
	onRefresh: () => void;
	onClose: () => void;
}

const EventRankingModal: React.FC<EventRankingModalProps> = ({
	isOpen,
	eventName,
	ranking,
	allianceId,
	isRefreshing,
	error,
	onRefresh,
	onClose,
}) => {
	const entries = useMemo(() => [...(ranking?.entries ?? [])].sort((left, right) => left.rank - right.rank), [ranking?.entries]);
	const loading = isRefreshing || ranking?.pending === true;

	return (
		<Modal
			isOpen={isOpen}
			onClose={onClose}
			maxWidth="6xl"
			title={`${eventName} alliance ranking`}
			footer={(
				<div className="flex w-full items-center justify-between gap-3">
					<p className="text-xs text-text-muted">Live data returned directly by the GGE event leaderboard.</p>
					<div className="flex gap-2">
						<Button type="button" variant="ghost" onClick={onClose}>Close</Button>
						<Button type="button" variant="primary" leftIcon={<RefreshCw className="h-4 w-4" />} isLoading={loading} onClick={onRefresh}>Refresh</Button>
					</div>
				</div>
			)}
		>
			<div className="flex flex-col gap-4">
				<div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
					<MetricTile label="Rows returned" value={entries.length} tone="brand" />
					<MetricTile label="Alliances" value={ranking?.totalAlliances ?? 0} />
					<MetricTile label="League ID" value={ranking?.leagueId ?? '—'} />
					<MetricTile label="List type" value={ranking?.listType ?? '—'} />
					<MetricTile label="First rank" value={formatOptionalNumber(ranking?.firstRank)} />
					<MetricTile label="Search value" value={ranking?.searchValue || '—'} />
					<MetricTile label="Global flag" value={ranking?.globalFlag ?? '—'} />
					<MetricTile label="Refreshed" value={formatObservedAt(ranking?.observedAt)} monospace={false} />
				</div>

				{error && (
					<div className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm text-error">{error}</div>
				)}

				{entries.length === 0 ? (
					<EmptyState
						size="md"
						icon={<Trophy className="h-7 w-7" />}
						title={loading ? 'Loading the GGE ranking…' : 'No ranking rows returned'}
						description={loading ? 'The game is returning the rows around your alliance position.' : 'Refresh after the Nomad alliance leaderboard becomes available.'}
					/>
				) : (
					<div className="overflow-x-auto rounded-global border border-border-light bg-bg-card/30 custom-scrollbar">
						<table className="w-full min-w-[780px] text-left text-sm">
							<thead className="border-b border-border-base bg-bg-card/80 text-[10px] font-bold uppercase tracking-wider text-text-muted">
								<tr>
									<th className="px-4 py-3 text-right">Rank</th>
									<th className="px-4 py-3">Alliance</th>
									<th className="px-4 py-3 text-right">Score</th>
									<th className="px-4 py-3 text-right">Members</th>
									<th className="px-4 py-3 text-right">Alliance fame</th>
									<th className="px-4 py-3 text-right">Alliance ID</th>
								</tr>
							</thead>
							<tbody className="divide-y divide-border-base/60">
								{entries.map((entry) => {
									const ownRow = Boolean(
										(entry.allianceId && entry.allianceId === ranking?.ownAllianceId)
										|| (allianceId && entry.allianceId === allianceId),
									);
									return (
										<tr key={entry.allianceId || `${entry.rank}-${entry.alliance}`} className={ownRow ? 'bg-primary/10' : 'hover:bg-bg-card-hover/45'}>
											<td className="px-4 py-3 text-right font-mono font-black tabular-nums text-primary">#{entry.rank.toLocaleString()}</td>
											<td className="px-4 py-3 font-semibold text-text-main">
												<div className="flex items-center gap-2">{entry.alliance || 'Unknown'}{ownRow && <Badge variant="primary">Your alliance</Badge>}</div>
											</td>
											<td className="px-4 py-3 text-right font-mono font-bold tabular-nums text-text-main">{entry.score.toLocaleString()}</td>
											<td className="px-4 py-3 text-right font-mono tabular-nums text-text-muted">{formatOptionalNumber(entry.memberCount)}</td>
											<td className="px-4 py-3 text-right font-mono tabular-nums text-text-muted">{formatOptionalNumber(entry.famePoints)}</td>
											<td className="px-4 py-3 text-right font-mono tabular-nums text-text-muted">{formatOptionalNumber(entry.allianceId)}</td>
										</tr>
									);
								})}
							</tbody>
						</table>
					</div>
				)}

				<p className="text-xs text-text-muted">
					GGE returns a window around the current alliance. The total reflects the full leaderboard, and every top-level and row field returned by the Nomad ranking endpoint is shown above.
				</p>
			</div>
		</Modal>
	);
};

function formatOptionalNumber(value?: number): string {
	return (value ?? 0) > 0 ? (value as number).toLocaleString() : '—';
}

function formatObservedAt(value?: string): string {
	if (!value) return 'Not yet';
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return 'Unknown';
	return date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit', second: '2-digit' });
}

export default EventRankingModal;
