import { useEffect, useMemo, useState } from 'react';
import { Binoculars, RefreshCw, Shield, Swords } from 'lucide-react';
import UnitImage from '../../components/UnitImage';
import DetailBackButton from '../../components/DetailBackButton';
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle, PageHeader } from '../../components/ui';
import { useMetadata, type MetadataItem } from '../../context/MetadataContext';

interface SpyPlayer { id?: number; name?: string; alliance?: string }
interface SpyCastle {
  id?: number;
  name?: string;
  kingdomID?: number;
  x?: number;
  y?: number;
  wallLevel?: number;
  gateLevel?: number;
  moatLevel?: number;
  keepLevel?: number;
  towerLevel?: number;
}
interface SpyUnit { wodID: number; amount: number }
interface SpySection { index: number; name: string; units: SpyUnit[]; total: number }
interface SpyCastellan {
  level?: number;
  generalID?: number;
  effects?: unknown[][];
  equipment?: unknown[][];
  skillIDs?: unknown[];
  calculatedEffects?: SpyEffect[];
}
interface SpyEffect { label?: string; name?: string; formattedValue?: string; value?: number; category?: string; sortOrder?: number }
export interface SpyReport {
  id: string;
  mid: number;
  capturedAtUnixMillis: number;
  status: 'success' | 'partial' | 'failed';
  accuracy?: number;
  risk?: number;
  spyCount?: number;
  guardCount?: number;
  target: SpyPlayer;
  source: SpyPlayer;
  castle: SpyCastle;
  setup?: SpySection[];
  totalTroops?: number;
  castellan?: SpyCastellan;
}

type UnitRole = 'attacker' | 'defender' | 'unknown';

const SpyReportsView = () => {
  const [reports, setReports] = useState<SpyReport[]>([]);
  const [selected, setSelected] = useState<SpyReport | null>(null);
  const [loading, setLoading] = useState(true);

  const load = async () => {
    setLoading(true);
    try {
	  const response = await fetch('/api/v2/history/spy-reports', { cache: 'no-cache' });
      if (!response.ok) throw new Error(`Spy reports request failed (${response.status})`);
      const payload = await response.json();
      setReports(Array.isArray(payload) ? payload : []);
    } catch (reason) {
      Notifications.error(reason instanceof Error ? reason.message : 'Could not load spy reports.', 'spy-reports-load');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  if (selected) {
    return <SpyReportDetail report={selected} onBack={() => setSelected(null)} />;
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Spy Reports"
        description="Review successful, partial, and failed espionage attempts."
        icon={<Binoculars className="h-6 w-6" />}
        actions={<Button variant="secondary" onClick={() => void load()} isLoading={loading} leftIcon={<RefreshCw className="h-4 w-4" />}>Refresh</Button>}
      />

      <Card>
        <CardHeader><CardTitle>Intelligence archive</CardTitle></CardHeader>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[860px] text-left text-sm">
            <thead className="border-b border-border-base bg-bg-card/25 text-xs uppercase text-text-muted">
              <tr><th className="px-5 py-3">Target</th><th className="px-4 py-3">Castle</th><th className="px-4 py-3">Result</th><th className="px-4 py-3 text-right">Troops seen</th><th className="px-4 py-3 text-right">Accuracy</th><th className="px-5 py-3">Captured</th></tr>
            </thead>
            <tbody className="divide-y divide-border-base/70">
              {reports.map((report) => (
                <tr key={report.id} className="cursor-pointer transition-colors hover:bg-bg-card-hover/45" onClick={() => setSelected(report)}>
                  <td className="px-5 py-3"><div className="font-semibold text-text-main">{report.target.name || 'Unknown player'}</div><div className="text-xs text-text-muted">{report.target.alliance || 'No alliance'}</div></td>
                  <td className="px-4 py-3"><div className="font-medium">{report.castle.name || `Castle ${report.castle.id || '—'}`}</div><div className="text-xs text-text-muted">{coordinateLabel(report.castle)}</div></td>
                  <td className="px-4 py-3"><StatusBadge status={report.status} /></td>
                  <td className="px-4 py-3 text-right font-semibold tabular-nums">{report.status === 'failed' ? '—' : formatNumber(report.totalTroops)}</td>
                  <td className="px-4 py-3 text-right tabular-nums">{report.accuracy != null ? `${report.accuracy}%` : '—'}</td>
                  <td className="px-5 py-3 text-text-muted">{formatDate(report.capturedAtUnixMillis)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {!loading && reports.length === 0 && <div className="px-5 py-12 text-center text-sm text-text-muted">No spy reports have been captured yet. New reports are collected when espionage notifications arrive.</div>}
      </Card>
    </div>
  );
};

export const SpyReportDetail = ({ report, onBack }: { report: SpyReport; onBack: () => void }) => {
  const { getTroop } = useMetadata();
  const estimates = useMemo(() => estimateRoles(report.setup ?? [], getTroop), [getTroop, report.setup]);

  return (
    <div className="space-y-5">
      <Card variant="solid" className="liquid-prominent-header-card">
        <CardHeader className="liquid-card-header-prominent flex-wrap gap-3">
          <div>
            <div className="text-xs font-semibold uppercase tracking-widest text-primary">Spy report dossier</div>
            <CardTitle className="mt-1">{report.castle.name || 'Unknown castle'}</CardTitle>
            <p className="mt-1 text-xs font-semibold text-text-muted">{report.target.name || 'Unknown player'} · {report.target.alliance || 'No alliance'} · {coordinateLabel(report.castle)}</p>
          </div>
          <DetailBackButton label="Back to alliance targets" onClick={onBack} />
        </CardHeader>
        <CardContent className="liquid-prominent-header-content">
          <TroopCompositionPanel estimates={estimates} />
        </CardContent>
      </Card>

      {report.status === 'failed' ? (
        <Card><CardContent className="p-8 text-center"><Shield className="mx-auto h-10 w-10 text-error" /><h2 className="mt-3 text-lg font-semibold">Espionage failed</h2><p className="mt-1 text-sm text-text-muted">The spies returned no usable troop or castle setup intelligence.</p></CardContent></Card>
      ) : (
        <>
          <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_22rem]">
            <Card variant="solid" className="liquid-prominent-header-card">
              <CardHeader className="liquid-card-header-prominent"><div><CardTitle>Castle troop setup</CardTitle><p className="mt-1 text-xs font-semibold text-text-muted">Observed positions and unit counts returned by the spy report.</p></div></CardHeader>
              <CardContent className="liquid-prominent-header-content grid gap-4 lg:grid-cols-2">
                {(report.setup ?? []).map((section) => <SetupSectionCard key={section.index} section={section} getTroop={getTroop} />)}
              </CardContent>
            </Card>

            {report.castellan && <CastellanPanel castellan={report.castellan} castle={report.castle} />}
          </div>
        </>
      )}
    </div>
  );
};

const SetupSectionCard = ({ section, getTroop }: { section: SpySection; getTroop: (id: number) => MetadataItem | undefined }) => (
  <div className="rounded-global border border-border-base bg-bg-app/35 p-4">
    <div className="mb-3 flex items-center justify-between"><h3 className="font-semibold">{section.name}</h3><Badge variant="secondary">{formatNumber(section.total)}</Badge></div>
    <div className="space-y-2">
      {section.units.map((unit, index) => {
        const meta = getTroop(unit.wodID);
        const role = classifyUnit(meta);
        return <div key={`${unit.wodID}-${index}`} className="flex items-center gap-3 rounded-global bg-bg-card/60 p-2"><UnitImage unitId={unit.wodID} size={38} showLevel /><div className="min-w-0 flex-1"><div className="truncate text-sm font-medium">{meta?.name || `Unit ${unit.wodID}`}</div><div className="text-xs capitalize text-text-muted">{role === 'unknown' ? 'Unknown role' : `${role}-oriented`}</div></div><div className="font-semibold tabular-nums">{formatNumber(unit.amount)}</div></div>;
      })}
    </div>
  </div>
);

const CastellanPanel = ({ castellan, castle }: { castellan: SpyCastellan; castle: SpyCastle }) => {
  const effects = (castellan.calculatedEffects ?? []).slice().sort((left, right) => (left.sortOrder ?? 900) - (right.sortOrder ?? 900));
  return <Card variant="solid" className="liquid-prominent-header-card xl:sticky xl:top-4"><CardHeader className="liquid-card-header-prominent"><div><CardTitle>Defense setup</CardTitle><p className="mt-1 text-xs font-semibold text-text-muted">Observed castellan and fortification levels.</p></div></CardHeader><CardContent className="liquid-prominent-header-content space-y-4"><div className="grid grid-cols-2 gap-2"><DefenseStat label="Castellan level" value={castellan.level} /><DefenseStat label="General" value={castellan.generalID} /><DefenseStat label="Wall level" value={castle.wallLevel} /><DefenseStat label="Gate level" value={castle.gateLevel} /><DefenseStat label="Moat level" value={castle.moatLevel} /><DefenseStat label="Keep level" value={castle.keepLevel} /></div><div className="space-y-2">{effects.map((effect, index) => <div key={`${effect.label}-${index}`} className="rounded-global border border-border-base bg-bg-app/35 p-3"><div className="flex items-start justify-between gap-3"><span className="text-xs font-semibold text-text-main">{effect.label || effect.name || 'Unknown effect'}</span><span className="shrink-0 text-sm font-bold tabular-nums text-primary">{effect.formattedValue || formatEffectValue(effect.value)}</span></div>{effect.category && <div className="mt-1 text-[10px] uppercase tracking-wide text-text-muted">{effect.category}</div>}</div>)}</div></CardContent></Card>;
};

const StatusBadge = ({ status }: { status: SpyReport['status'] }) => <Badge variant={status === 'success' ? 'success' : status === 'failed' ? 'danger' : 'warning'}>{status === 'success' ? 'Successful' : status === 'partial' ? 'Partial intel' : 'Failed'}</Badge>;
const TroopCompositionPanel = ({ estimates }: { estimates: { attackers: number; defenders: number; unknown: number } }) => <div><div className="grid gap-3 md:grid-cols-[1fr_auto_1fr]"><CompositionSide label="Attack-oriented" value={estimates.attackers} icon={<Swords className="h-5 w-5" />} tone="text-error" /><div className="hidden items-center text-xs font-black uppercase tracking-widest text-text-muted md:flex">vs</div><CompositionSide label="Defense-oriented" value={estimates.defenders} icon={<Shield className="h-5 w-5" />} tone="text-primary" /></div>{estimates.unknown > 0 && <div className="mt-2 text-center text-xs text-text-muted">{formatNumber(estimates.unknown)} units could not be classified</div>}</div>;
const CompositionSide = ({ label, value, icon, tone }: { label: string; value: number; icon: React.ReactNode; tone: string }) => <div className="flex items-center gap-3 rounded-global border border-border-base bg-bg-app/30 p-4"><div className={tone}>{icon}</div><div><div className="text-xs font-semibold uppercase tracking-wide text-text-muted">{label}</div><div className="mt-1 text-2xl font-bold tabular-nums">{formatNumber(value)}</div></div></div>;
const DefenseStat = ({ label, value }: { label: string; value?: number }) => <div className="rounded-global bg-bg-app/40 p-2.5"><div className="text-[10px] uppercase tracking-wide text-text-muted">{label}</div><div className="mt-1 text-base font-bold tabular-nums">{value ?? '—'}</div></div>;

function estimateRoles(sections: SpySection[], getTroop: (id: number) => MetadataItem | undefined) {
  const totals = { attackers: 0, defenders: 0, unknown: 0 };
  for (const section of sections) for (const unit of section.units) {
    const role = classifyUnit(getTroop(unit.wodID));
    if (role === 'attacker') totals.attackers += unit.amount;
    else if (role === 'defender') totals.defenders += unit.amount;
    else totals.unknown += unit.amount;
  }
  return totals;
}

function classifyUnit(meta?: MetadataItem): UnitRole {
  if (!meta) return 'unknown';
  const attack = Math.max(numberValue(meta.meleeAttack), numberValue(meta.rangeAttack));
  const defense = Math.max(numberValue(meta.meleeDefence), numberValue(meta.rangeDefence), numberValue(meta.meleeDefense), numberValue(meta.rangeDefense));
  if (attack <= 0 && defense <= 0) return 'unknown';
  return attack > defense ? 'attacker' : 'defender';
}

function formatEffectValue(value?: number): string { if (value == null) return '—'; return `${value > 0 ? '+' : ''}${value.toFixed(1)}%`; }
function numberValue(value: unknown): number { const result = Number(value); return Number.isFinite(result) ? result : 0; }
function formatNumber(value?: number): string { return new Intl.NumberFormat().format(value || 0); }
function formatDate(value: number): string { const date = new Date(value); return Number.isNaN(date.getTime()) ? 'Unknown' : date.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' }); }
function coordinateLabel(castle: SpyCastle): string { return castle.x != null && castle.y != null ? `${castle.x}:${castle.y} · Kingdom ${castle.kingdomID ?? 0}` : 'Unknown'; }

export default SpyReportsView;
