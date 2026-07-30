import { useMemo } from 'react';
import { useCitadelAPI } from '../../api/ApiContext';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import { Icons } from '../../components/Icons';
import { SectionCard } from '../../components/ui';
import { useMetadata, type MetadataItem } from '../../context/MetadataContext';

interface DefinitionAmount {
	id: number;
	name: string;
	code?: string;
	image?: string;
	amount: number;
}

export default function CurrencyView() {
	const { state } = useCitadelAPI();
	const { resources, currencies, isLoading } = useMetadata();
	const groups = useMemo(() => {
		if (!state) return [];
		return [
			{
				name: 'Resources',
				items: definitionAmounts(state.player.resources, resources),
			},
			{
				name: 'Currencies',
				items: definitionAmounts(state.player.currencies, currencies),
			},
			{
				name: 'Account Stats',
				items: [
					{ id: 1, name: 'Might', amount: state.player.might ?? 0 },
					{ id: 2, name: 'Glory', amount: state.player.glory ?? 0 },
					{ id: 3, name: 'Gallantry', amount: state.player.gallantry ?? 0 },
				].filter((item) => item.amount !== 0),
			},
		].filter((group) => group.items.length > 0);
	}, [currencies, resources, state]);

	if (!state || isLoading) {
		return (
			<div className="flex flex-col gap-6 h-full items-center justify-center">
				<StaleSessionBanner />
				<div className="text-primary animate-pulse">Loading official resource definitions…</div>
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-8 pb-8">
			<StaleSessionBanner />
			{groups.map((group) => (
				<SectionCard key={group.name} variant="glass" title={group.name} titleClassName="text-xl text-primary" className="flex flex-col" contentClassName="p-6">
					<div className="currency-responsive-grid">
						{group.items.map((item) => <AmountCard key={`${group.name}-${item.id}`} item={item} />)}
					</div>
				</SectionCard>
			))}
		</div>
	);
}

function AmountCard({ item }: { item: DefinitionAmount }) {
	return (
		<div className="flex flex-col items-center justify-center p-4 gap-3 bg-bg-card border border-border-base rounded-global shadow-sm hover:border-primary/50 hover:bg-bg-card-hover transition-colors">
			<span className="text-xs font-bold text-text-muted uppercase tracking-wider text-center h-8 flex items-center justify-center">
				{item.name}
			</span>
			<div className="h-14 flex items-center justify-center">
				{item.image ? (
					<img src={item.image} alt="" loading="lazy" decoding="async" className="w-12 h-12 object-contain drop-shadow-md" />
				) : (
					<Icons.Database className="w-10 h-10 text-primary" />
				)}
			</div>
			<span className="text-lg font-bold text-text-main font-mono mt-1">{item.amount.toLocaleString()}</span>
			{item.code && <span className="text-[10px] text-text-muted font-mono">{item.code}</span>}
		</div>
	);
}

function definitionAmounts(
	amounts: Record<string, number>,
	definitions: Record<number, MetadataItem>,
): DefinitionAmount[] {
	return Object.entries(amounts)
		.map(([rawID, amount]) => {
			const id = Number(rawID);
			const definition = definitions[id];
			return {
				id,
				name: definition?.name ?? `Definition ${id}`,
				code: typeof definition?.JSONKey === 'string' ? definition.JSONKey : undefined,
				image: definition?.image,
				amount,
			};
		})
		.sort((left, right) => left.name.localeCompare(right.name) || left.id-right.id);
}
