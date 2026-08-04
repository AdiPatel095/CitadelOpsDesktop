import React, { useMemo } from 'react';
import { Coins, Crosshair, Database, Shield, Sparkles, Swords } from 'lucide-react';
import type { EventActivityStateV2, EventCombatTotalsV2, ScalableEventScoreV2 } from '../../api/Contracts';
import { useCitadelAPI } from '../../api/ApiContext';
import { MetricTile, SectionCard } from '../../components/ui';
import { useMetadata } from '../../context/MetadataContext';

interface EventActivityCardProps {
	event?: ScalableEventScoreV2;
}

interface ActivityGroup {
	key: string;
	label: string;
	description: string;
	firstMetricLabel: string;
	totals: EventCombatTotalsV2;
	icon: React.ReactNode;
}

type EventFamily = 'nomad' | 'samurai' | 'foreignLords' | 'bloodcrow' | 'other';

const EMPTY_TOTALS: EventCombatTotalsV2 = {
	launches: 0,
	battles: 0,
	victories: 0,
	defeats: 0,
	troopLosses: 0,
	toolsUsed: 0,
	loot: 0,
};

const EVENT_CURRENCIES: Record<number, number[]> = {
	72: [1, 10, 77],
	80: [7, 13, 78],
};

const CURRENCY_FALLBACK_NAMES: Record<number, string> = {
	1: 'Khan tablets',
	7: 'Samurai tokens',
	10: 'Khan medals',
	13: 'Samurai medals',
	77: 'Nomad Advisor tokens',
	78: 'Samurai Advisor tokens',
};

const EventActivityCard: React.FC<EventActivityCardProps> = ({ event }) => {
	const { state } = useCitadelAPI();
	const { currencies } = useMetadata();
	const activity = event ? state?.eventScores.activityByEvent?.[String(event.eventId)] : undefined;
	const family = event ? eventFamily(event) : 'other';
	const currencyIDs = useMemo(() => {
		if (!event) return [];
		const IDs = new Set(EVENT_CURRENCIES[event.eventId] ?? []);
		if ((event.advisorCurrencyId ?? 0) > 0) IDs.add(event.advisorCurrencyId as number);
		return Array.from(IDs);
	}, [event]);
	const groups = activityGroups(activity, family);
	const outbound = sumTotals(groups.filter((group) => group.key !== 'khanDefense').map((group) => group.totals));
	const combined = sumTotals(groups.map((group) => group.totals));
	const includesDefense = groups.some((group) => group.key === 'khanDefense');
	const showsLoot = family === 'foreignLords' || family === 'bloodcrow';

	if (!event || (currencyIDs.length === 0 && groups.length === 0)) return null;

	return (
		<>
			{currencyIDs.length > 0 && <SectionCard
				variant="solid"
				title="Event balances"
				description="Current currencies used by this event"
				icon={<Coins className="h-5 w-5" />}
			>
				<div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
						{currencyIDs.map((currencyID) => {
							const metadata = currencies[currencyID];
							const amount = state?.player.currencies[String(currencyID)] ?? 0;
							return (
								<div key={currencyID} className="flex items-center gap-3 rounded-global border border-border-light bg-bg-card/40 p-3">
									<div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-global border border-border-base bg-bg-input/55">
										{metadata?.image ? (
											<img src={metadata.image} alt="" loading="lazy" decoding="async" className="h-10 w-10 object-contain drop-shadow-md" />
										) : (
											<Database className="h-7 w-7 text-primary" />
										)}
									</div>
									<div className="min-w-0">
										<p className="truncate text-xs font-bold uppercase tracking-wider text-text-muted">{metadata?.name ?? CURRENCY_FALLBACK_NAMES[currencyID] ?? `Currency ${currencyID}`}</p>
										<p className="mt-1 font-mono text-xl font-black tabular-nums text-text-main">{amount.toLocaleString()}</p>
									</div>
								</div>
							);
						})}
					</div>
			</SectionCard>}

			{groups.length > 0 && <SectionCard
				variant="solid"
				title="Automation results"
				description={activity?.observedFrom ? `Observed since ${formatDateTime(activity.observedFrom)}` : 'Current event occurrence'}
				icon={<Sparkles className="h-5 w-5" />}
			>
				<div className={`grid grid-cols-2 gap-3 ${showsLoot ? 'lg:grid-cols-5' : 'lg:grid-cols-4'}`}>
					<MetricTile label="Attacks made" value={observedActions(outbound)} tone="brand" caption={attackCaption(family)} />
					<MetricTile label="Victories" value={combined.victories} tone="success" caption={includesDefense ? 'Offense and Khan defense' : 'Confirmed battle reports'} />
					{showsLoot && <MetricTile label="Loot" value={combined.loot} tone="warning" caption="Resources from confirmed reports" />}
					<MetricTile label="Troops lost" value={combined.troopLosses} tone={combined.troopLosses > 0 ? 'danger' : 'default'} />
					<MetricTile label="Tools used" value={combined.toolsUsed} tone="info" />
				</div>

				<div className="mt-4 grid gap-3 xl:grid-cols-2">
					{groups.map((group) => <ActivityGroupCard key={group.key} group={group} />)}
				</div>
				<p className="mt-4 border-t border-border-base/60 pt-3 text-xs text-text-muted">
					{activityFootnote(family)}
				</p>
			</SectionCard>}
		</>
	);
};

function ActivityGroupCard({ group }: { group: ActivityGroup }) {
	const totals = group.totals;
	return (
		<div className="rounded-global border border-border-light bg-bg-card/40 p-4">
			<div className="flex items-start gap-3">
				<span className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-global border border-primary/20 bg-primary/10 text-primary" aria-hidden="true">
					{group.icon}
				</span>
				<div className="min-w-0">
					<h3 className="font-bold text-text-main">{group.label}</h3>
					<p className="mt-0.5 text-xs text-text-muted">{group.description}</p>
				</div>
			</div>
			<div className={`mt-4 grid gap-x-3 gap-y-4 ${group.key === 'invasion' ? 'grid-cols-2 sm:grid-cols-4' : 'grid-cols-3'}`}>
				<CompactMetric label={group.firstMetricLabel} value={observedActions(totals)} />
				<CompactMetric label="Resolved" value={totals.battles} />
				<CompactMetric label="Victories" value={totals.victories} tone="text-success" />
				<CompactMetric label="Defeats" value={totals.defeats} tone={totals.defeats > 0 ? 'text-error' : undefined} />
				<CompactMetric label="Troops lost" value={totals.troopLosses} tone={totals.troopLosses > 0 ? 'text-error' : undefined} />
				<CompactMetric label="Tools used" value={totals.toolsUsed} />
				{group.key === 'invasion' && <CompactMetric label="Loot" value={totals.loot} tone="text-warning" />}
			</div>
		</div>
	);
}

function CompactMetric({ label, value, tone = 'text-text-main' }: { label: string; value: number; tone?: string }) {
	return (
		<div className="min-w-0">
			<p className="truncate text-[9px] font-bold uppercase tracking-wider text-text-muted">{label}</p>
			<p className={`mt-1 font-mono text-base font-black tabular-nums ${tone}`}>{value.toLocaleString()}</p>
		</div>
	);
}

function activityGroups(activity: EventActivityStateV2 | undefined, family: EventFamily): ActivityGroup[] {
	if (family === 'foreignLords' || family === 'bloodcrow') {
		return [{
			key: 'invasion',
			label: family === 'bloodcrow' ? 'Bloodcrow attacks' : 'Foreign Lord attacks',
			description: `${family === 'bloodcrow' ? 'Bloodcrow' : 'Foreign Lord'} castles launched by Auto Invasion`,
			firstMetricLabel: 'Attacks',
			totals: activity?.invasion ?? EMPTY_TOTALS,
			icon: <Crosshair className="h-4 w-4" />,
		}];
	}
	if (family !== 'nomad' && family !== 'samurai') return [];
	const isNomad = family === 'nomad';
	const groups: ActivityGroup[] = [
		{
			key: 'camp', label: isNomad ? 'Nomad camp attacks' : 'Samurai camp attacks',
			description: isNomad ? 'Regular Nomad camps launched by Auto Nomad' : 'Regular Samurai camps launched by Auto Nomad', firstMetricLabel: 'Attacks',
			totals: activity?.camp ?? EMPTY_TOTALS, icon: <Crosshair className="h-4 w-4" />,
		},
		{
			key: 'advisor', label: 'Advisor camp attacks', description: 'Repeated event-camp attacks run by the in-game Advisor', firstMetricLabel: 'Attacks',
			totals: activity?.advisor ?? EMPTY_TOTALS, icon: <Sparkles className="h-4 w-4" />,
		},
	];
	if (isNomad) {
		groups.push(
			{
				key: 'khan', label: 'Khan attacks', description: 'Outbound attacks against the Khan camp', firstMetricLabel: 'Attacks',
				totals: activity?.khan ?? EMPTY_TOTALS, icon: <Swords className="h-4 w-4" />,
			},
			{
				key: 'khanDefense', label: 'Khan defenses', description: 'Incoming Khan attacks defended by your castles', firstMetricLabel: 'Incoming',
				totals: activity?.khanDefense ?? EMPTY_TOTALS, icon: <Shield className="h-4 w-4" />,
			},
		);
	}
	return groups;
}

function attackCaption(family: EventFamily): string {
	if (family === 'nomad') return 'Camp, Advisor, and Khan offense';
	if (family === 'samurai') return 'Camp and Advisor attacks';
	return 'Auto Invasion attacks';
}

function activityFootnote(family: EventFamily): string {
	if (family === 'nomad') {
		return 'Counts come from successful CitadelOps launches and confirmed battle reports for this event occurrence. Khan defenses are tracked separately from attacks made.';
	}
	return 'Counts come from successful CitadelOps launches and confirmed battle reports for this event occurrence.';
}

function sumTotals(totals: EventCombatTotalsV2[]): EventCombatTotalsV2 {
	return totals.reduce((sum, current) => ({
		launches: sum.launches + current.launches,
		battles: sum.battles + current.battles,
		victories: sum.victories + current.victories,
		defeats: sum.defeats + current.defeats,
		troopLosses: sum.troopLosses + current.troopLosses,
		toolsUsed: sum.toolsUsed + current.toolsUsed,
		loot: sum.loot + current.loot,
	}), { ...EMPTY_TOTALS });
}

function observedActions(totals: EventCombatTotalsV2): number {
	return Math.max(totals.launches, totals.battles);
}

function eventFamily(event: ScalableEventScoreV2): EventFamily {
	const identity = eventIdentity(event);
	if (event.eventId === 72 || identity.includes('nomad')) return 'nomad';
	if (event.eventId === 80 || identity.includes('samurai')) return 'samurai';
	if (event.eventId === 103 || identity.includes('bloodcrow') || identity.includes('red alliance alien')) return 'bloodcrow';
	if (event.eventId === 71 || identity.includes('alien invasion')) return 'foreignLords';
	return 'other';
}

function eventIdentity(event: ScalableEventScoreV2): string {
	return `${event.eventType ?? ''} ${event.name ?? ''} ${event.localizationKey ?? ''}`.toLowerCase();
}

function formatDateTime(value: string): string {
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return 'this app session';
	return date.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' });
}

export default EventActivityCard;
