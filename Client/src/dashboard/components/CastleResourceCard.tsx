import React, { useEffect, useState } from 'react';
import { Card, CardHeader, CardTitle, CardContent } from '../../components/ui';

const WoodIcon = '/game-data/resources/images/Wood.webp';
const StoneIcon = '/game-data/resources/images/Stone.webp';
const FoodIcon = '/game-data/resources/images/Food.webp';
const CharcoalIcon = '/game-data/resources/images/Charcoal.webp';
const OliveOilIcon = '/game-data/resources/images/OliveOil.webp';
const GlassIcon = '/game-data/resources/images/Glass.webp';
const IronOreIcon = '/game-data/resources/images/Iron_Ore.webp';
const HoneyIcon = '/game-data/resources/images/Honey.webp';
const MeadIcon = '/game-data/resources/images/Mead.webp';
const BeefIcon = '/game-data/resources/images/Beef.webp';

import {
  type CastleResourcesAmount,
  type CastleStorageMax,
  type CastleProductionTotal
} from '../models/PlayerCastleInfo';

interface CastleResourceCardProps {
  title: string;
  resources: CastleResourcesAmount;
  storage: CastleStorageMax;
  production: CastleProductionTotal;
}

const resourceIconMap: { [key: string]: string } = {
  wood: WoodIcon,
  stone: StoneIcon,
  food: FoodIcon,
  coal: CharcoalIcon,
  oil: OliveOilIcon,
  glass: GlassIcon,
  iron: IronOreIcon,
  honey: HoneyIcon,
  mead: MeadIcon,
  beef: BeefIcon,
};

/** Display order: beef → mead → food → honey → charcoal (coal) → oil → glass → iron → wood → stone */
const resourceKeys: (keyof CastleResourcesAmount)[] = [
  'beef_amount',
  'mead_amount',
  'food_amount',
  'honey_amount',
  'coal_amount',
  'oil_amount',
  'glass_amount',
  'iron_amount',
  'wood_amount',
  'stone_amount',
];

const MS_PER_HOUR = 3600 * 1000;
const EXTREME_HOURS = 24 * 365 * 10;

const SEC_PER_HOUR = 3600;
const SEC_PER_DAY = 86400;

/** Live countdown: d/h/m when at least 1 hour left; m/s when under 1 hour. */
function formatRemainingMs(ms: number): string {
  const totalSec = Math.max(0, Math.floor(ms / 1000));

  if (totalSec >= SEC_PER_HOUR) {
    const days = Math.floor(totalSec / SEC_PER_DAY);
    const rem = totalSec % SEC_PER_DAY;
    const hrs = Math.floor(rem / SEC_PER_HOUR);
    const mins = Math.floor((rem % SEC_PER_HOUR) / 60);
    if (days > 0) {
      const parts: string[] = [`${days}d`];
      if (hrs > 0) parts.push(`${hrs}h`);
      if (mins > 0) parts.push(`${mins}m`);
      return parts.join(' ');
    }
    if (mins > 0) return `${hrs}h ${mins}m`;
    return `${hrs}h`;
  }

  const mins = Math.floor(totalSec / 60);
  const secs = totalSec % 60;
  if (mins > 0) return `${mins}m ${secs}s`;
  return `${secs}s`;
}

/** Shown above the resource bar when net/hr is negative; time counts down until the next amount/prod update. */
function ResourceDepletionTimer({ amount, netPerHour }: { amount: number; netPerHour: number }) {
  const [deadlineMs, setDeadlineMs] = useState<number | null>(null);
  const [extremeLong, setExtremeLong] = useState(false);

  useEffect(() => {
    setExtremeLong(false);
    if (netPerHour >= -1e-9 || amount <= 0) {
      setDeadlineMs(null);
      return;
    }
    const hours = amount / (-netPerHour);
    if (!Number.isFinite(hours) || hours <= 0) {
      setDeadlineMs(null);
      return;
    }
    if (hours > EXTREME_HOURS) {
      setDeadlineMs(null);
      setExtremeLong(true);
      return;
    }
    setDeadlineMs(Date.now() + hours * MS_PER_HOUR);
  }, [amount, netPerHour]);

  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (deadlineMs == null) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [deadlineMs]);

  if (extremeLong) {
    return (
      <p className="text-[10px] text-text-muted leading-tight tabular-nums text-right w-full">&gt;10y</p>
    );
  }
  if (deadlineMs == null) return null;
  const rem = deadlineMs - now;
  if (rem <= 0) return null;

  return (
    <p className="text-[10px] text-text-muted leading-tight tabular-nums text-right w-full">
      {formatRemainingMs(rem)}
    </p>
  );
}

const CastleResourceCard: React.FC<CastleResourceCardProps> = ({ title, resources, storage, production }) => {
  return (
    <Card className="liquid-prominent-header-card flex flex-col min-h-0">
      <CardHeader className="liquid-card-header-prominent">
        <CardTitle className="text-primary">{title}</CardTitle>
      </CardHeader>
      <CardContent className="liquid-prominent-header-content flex flex-col gap-2 overflow-y-auto custom-scrollbar">
        {resourceKeys.map(key => {
          const resourceBaseName = key.replace('_amount', '');
          const amount = resources[key] as number;
          const max = storage[`${resourceBaseName}_max` as keyof CastleStorageMax] as number;
          let prod = production[`${resourceBaseName}_prod` as keyof CastleProductionTotal] ?? 0;

          // Deduct consumption for food/mead/beef
          if (resourceBaseName === 'food') {
            prod -= (production.food_consumption ?? 0);
          } else if (resourceBaseName === 'mead') {
            prod -= (production.mead_consumption ?? 0);
          } else if (resourceBaseName === 'beef') {
            prod -= (production.beef_consumption ?? 0);
          }

          const percentage = max > 0 ? (amount / max) * 100 : 0;
          const prodClass = prod < 0 ? "text-error font-semibold" : "text-success font-semibold";
          const prodPrefix = prod > 0 ? "+" : "";

          return (
            <div key={key} className="flex items-center gap-3 p-2.5 rounded-global bg-bg-card/45 border border-border-light shadow-sm backdrop-blur-xl transition-colors hover:border-primary/30 hover:bg-bg-card-hover/70">
              <img src={resourceIconMap[resourceBaseName]} alt={resourceBaseName} className="w-8 h-8 object-contain drop-shadow-sm shrink-0" />
              <div className="flex-1 flex flex-col gap-1.5 min-w-0">
                <div className="flex justify-between items-center text-xs font-medium text-text-main">
                  <span className="truncate mr-2">{amount.toLocaleString()} / {max.toLocaleString()}</span>
                  <span className={`${prodClass} shrink-0`}>({prodPrefix}{prod.toLocaleString()}/hr)</span>
                </div>
                <ResourceDepletionTimer amount={amount} netPerHour={prod} />
                <div className="w-full h-1.5 bg-bg-app/55 rounded-full overflow-hidden border border-border-base/50 shadow-inner">
                  <div className="h-full bg-primary transition-all duration-500 ease-out" style={{ width: `${Math.min(100, Math.max(0, percentage))}%` }}></div>
                </div>
              </div>
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
};

export default CastleResourceCard;
