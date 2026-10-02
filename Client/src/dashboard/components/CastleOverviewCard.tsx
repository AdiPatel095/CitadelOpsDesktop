import { TriangleAlert } from 'lucide-react';
import './CastleOverviewCard.css';
import type { CastleStateV2 } from '../../api/Contracts';
import { useCitadelAPI } from '../../api/ApiContext';
import { useMetadata } from '../../context/MetadataContext';
import { parseAutoBeriWorldSettings } from '../../settings/AutoBeriWorldClientState';
import { useHostedRuntimePresence } from '../../config/Deployment';
import { requestView } from '../../config/viewNavigation';
import { useLocale } from '../../i18n/LocaleContext';
import { Button, MetricTile, SectionCard } from '../../components/ui';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { automationsActingOnCastle } from '../../settings/castleAutomations';
import type { SettingsFeatureId } from '../../settings/disclosure/placement';
import { useAutomationPlayerStatus } from '../../settings/readiness/useAutomationPlayerStatus';
import { castleDataAge, resourceRows, troopTotals } from '../castleOverview';

const featureNames: Partial<Record<SettingsFeatureId, string>> = {
  autoTowers: 'Auto Towers', autoRecruit: 'Auto Recruit', autoTool: 'Auto Tool',
  autoSceatRes: 'Auto Sceat Resources', autoStation: 'Auto Station', autoBird: 'Auto Bird',
  autoKhan: 'Auto Khan', autoNomad: 'Auto Nomad / Samurai', autoInvasion: 'Auto Invasion',
  autoAdvisor: 'Auto Advisor', autoBeriWorld: 'Auto Beri World', autoBuyer: 'Auto Buyer', autoStorm: 'Auto Storm',
};

function CastleAutomationRow({ featureId }: { featureId: SettingsFeatureId }) {
  const { configuration } = useCitadelAPI();
  const player = useAutomationPlayerStatus(featureId, {
    buildLaneActive: featureId === 'autoBeriWorld' ? parseAutoBeriWorldSettings(configuration?.sections?.['automation.autoBeriWorld']).build.enabled : undefined,
  });
  return <li className="castle-overview-automation">
    <span><bdi>{featureNames[featureId]}</bdi></span>
    <StatusBadge {...player.overall} />
  </li>;
}

export default function CastleOverviewCard({ castle }: { castle: CastleStateV2 }) {
  const { t, number, date } = useLocale();
  const { resources: definitions } = useMetadata();
  const { configuration, connectionStatus } = useCitadelAPI();
  const presence = useHostedRuntimePresence();
  const rows = resourceRows(castle, definitions);
  const troops = troopTotals(castle.units);
  const features = automationsActingOnCastle(configuration?.sections ?? {}, castle.id);
  const age = castleDataAge({ presence, connected: connectionStatus === 'Connected', castle });
  const formatNumber = (value: number) => number(value, { maximumFractionDigits: 1 });
  return <SectionCard title={t('castleOverview.title')} data-castle-overview contentClassName="castle-overview-content">
    {age.kind === 'saved' && <p className="castle-overview-saved">
      {age.at ? t('castleOverview.savedDataFrom', { time: date(new Date(age.at), { dateStyle: 'medium', timeStyle: 'short' }) }) : t('castleOverview.savedDataUnknown')}
    </p>}
    <section>
      <h3>{t('castleOverview.resources')}</h3>
      <div className="castle-overview-resources">
        {rows.filter(row => row.capacity != null && row.capacity > 0).map(row => <div key={row.id}>
          <div className="castle-overview-resource-label">
            {row.icon && <img src={row.icon} alt="" />}
            <span><bdi>{row.name}</bdi></span><span>{formatNumber(row.amount)} / {formatNumber(row.capacity!)}</span>
          </div>
          <div className="castle-overview-storage" role="meter" aria-label={row.name}
            aria-valuemin={0} aria-valuemax={Math.max(0, row.capacity!)} aria-valuenow={Math.max(0, Math.min(row.amount, row.capacity!))}
            aria-valuetext={`${formatNumber(row.amount)} / ${formatNumber(row.capacity!)}`} data-near-cap={row.nearCap}>
            <span style={{ width: `${Math.max(0, Math.min(1, row.ratio)) * 100}%` }} />
          </div>
          {row.nearCap && <span className="castle-overview-warning"><TriangleAlert aria-hidden="true" />{t('castleOverview.nearCap')}</span>}
        </div>)}
      </div>
    </section>
    <section>
      <h3>{t('castleOverview.production')}</h3>
      <div className="castle-overview-resources">
        {rows.map(row => <div className="castle-overview-resource-label" key={row.id}>
          {row.icon && <img src={row.icon} alt="" />}<span><bdi>{row.name}</bdi></span>
          <span className="castle-overview-production" data-consuming={row.perHour < 0}>
            {row.perHour > 0 ? '+' : row.perHour < 0 ? '−' : ''}{formatNumber(Math.abs(row.perHour))}
          </span>
        </div>)}
      </div>
    </section>
    <section>
      <h3>{t('castleOverview.troops')}</h3>
      <div className="castle-overview-troops">
        <MetricTile label={t('castleOverview.stationed')} value={troops.stationed} />
        <MetricTile label={t('castleOverview.traveling')} value={troops.traveling} />
        <MetricTile label={t('castleOverview.hospital')} value={troops.hospital} />
      </div>
    </section>
    <section>
      <h3>{t('castleOverview.automations')}</h3>
      {features.length ? <ul className="castle-overview-automations">{features.map(featureId => <CastleAutomationRow key={featureId} featureId={featureId} />)}</ul>
        : <p className="castle-overview-muted">{t('castleOverview.noAutomations')}</p>}
      <Button variant="ghost" size="md" onClick={() => requestView('automation')}>{t('castleOverview.seeAll')}</Button>
    </section>
  </SectionCard>;
}
