import React, { useEffect, useMemo, useState } from 'react';
import {
  ArrowRight,
  BarChart3,
  CalendarDays,
  Castle,
  ChevronDown,
  ChevronRight,
  MapPin,
  RefreshCw,
  Search,
  Shield,
  Swords,
  Users,
} from 'lucide-react';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle, Input, Select } from '../../components/ui';
import UnitImage from '../../components/UnitImage';
import ToolImage from '../../components/ToolImage';
import { useMetadata } from '../../context/MetadataContext';
import { useLastKnownSnapshot } from '../../context/LastKnownSnapshotContext';
import { FrontendWebsocket } from '../../Websocket';
import type {
  BattleCombatant,
  BattleEffect,
  BattleItemDetail,
  BattleMetrics,
  BattleStatsSummary,
  BattleWave,
  BattleWaveLane,
  ParsedReport,
} from '../types/BattleStats';

const dataSources = [
  '/api/battle-reports/cloud',
  '/api/battleReports/cloud',
  '/api/reports/battle',
  '/api/battle-reports',
  '/api/battleReports',
  '/Data/BattleReports.jsonl',
  '/BattleReports.jsonl',
];

const kingdomNames: Record<number, string> = {
  0: 'The Great Empire (Green)',
  1: 'The Burning Sands (Sand)',
  2: 'The Everwinter Glacier (Ice)',
  3: 'The Fire Peaks (Fire)',
  4: 'The Storm Islands (Storm)',
};

const allOption = 'all';

interface FilterOption {
  value: string;
  label: string;
}

interface AllianceMemberOption extends FilterOption {
  keys: string[];
}

type CombatantSide = 'attacker' | 'defender';

const BattleStatsView: React.FC = () => {
  const { snapshot } = useLastKnownSnapshot();
  const [reports, setReports] = useState<ParsedReport[]>([]);
  const [allianceInfo, setAllianceInfo] = useState<Record<string, unknown> | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [sourceLabel, setSourceLabel] = useState<string>('Not loaded');
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedPlayer, setSelectedPlayer] = useState(allOption);
  const [selectedOpponentPlayer, setSelectedOpponentPlayer] = useState(allOption);
  const [selectedAlliance, setSelectedAlliance] = useState(allOption);
  const [selectedResult, setSelectedResult] = useState(allOption);
  const [selectedRole, setSelectedRole] = useState(allOption);
  const [startDate, setStartDate] = useState(() => inputDateFromDaysAgo(90));
  const [endDate, setEndDate] = useState(() => inputDateFromDate(new Date()));
  const [selectedReportID, setSelectedReportID] = useState<string | null>(null);

  const loadReports = async () => {
    setIsLoading(true);
    setLoadError(null);

    try {
      let emptySource = '';
      for (const source of dataSources) {
        const loaded = await fetchReports(source);
        if (loaded) {
          if (loaded.reports.length === 0) {
            emptySource = emptySource || loaded.source;
            continue;
          }
          setReports(loaded.reports);
          setSourceLabel(loaded.source);
          setSelectedReportID((current) =>
            current && loaded.reports.some((report) => reportID(report) === current) ? current : null
          );
          return;
        }
      }

      setReports([]);
      setSourceLabel(emptySource ? `${emptySource} is empty` : 'No local archive found');
      setSelectedReportID(null);
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : 'Could not load battle reports');
      setReports([]);
      setSourceLabel('Load failed');
      setSelectedReportID(null);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    const handleMessage = (message: { type?: string; payload?: unknown }) => {
      if (message.type === 'allianceInfo' && isRecord(message.payload)) {
        setAllianceInfo(message.payload);
      }
    };

    FrontendWebsocket.addMessageListener(handleMessage);
    FrontendWebsocket.sendFetchAllianceInfo();
    void loadReports();

    return () => FrontendWebsocket.removeMessageListener(handleMessage);
  }, []);

  const allianceMembers = useMemo(
    () => allianceMemberOptionsFromSources(snapshot, allianceInfo),
    [snapshot, allianceInfo]
  );
  const allianceMemberKeys = useMemo(() => allianceMemberKeysFromOptions(allianceMembers), [allianceMembers]);
  const homeAllianceKey = useMemo(
    () => resolveHomeAllianceKey(reports, allianceMemberKeys),
    [reports, allianceMemberKeys]
  );

  const playerOptions = useMemo(
    () => buildAlliancePlayerOptions(reports, allianceMembers, allianceMemberKeys, homeAllianceKey),
    [reports, allianceMembers, allianceMemberKeys, homeAllianceKey]
  );

  const allianceOptions = useMemo(() => {
    const allianceOptionReports = reports
      .filter(hasBothPlayers)
      .filter((report) =>
        selectedOpponentPlayer === allOption ||
        reportMatchesOpponentPlayer(report, selectedPlayer, selectedOpponentPlayer, allianceMemberKeys, homeAllianceKey)
      );
    const rows = buildAllianceAggregates(
      allianceOptionReports,
      selectedPlayer,
      allianceMemberKeys,
      homeAllianceKey
    );

    return [
      { value: allOption, label: 'All opponent alliances' },
      ...rows.map((row) => ({ value: row.key, label: row.name })),
    ];
  }, [reports, selectedPlayer, selectedOpponentPlayer, allianceMemberKeys, homeAllianceKey]);

  const opponentPlayerOptions = useMemo(
    () => buildOpponentPlayerOptions(reports.filter(hasBothPlayers), selectedPlayer, selectedAlliance, allianceMemberKeys, homeAllianceKey),
    [reports, selectedPlayer, selectedAlliance, allianceMemberKeys, homeAllianceKey]
  );

  useEffect(() => {
    if (selectedPlayer !== allOption && !filterOptionExists(playerOptions, selectedPlayer)) {
      setSelectedPlayer(allOption);
    }
  }, [playerOptions, selectedPlayer]);

  useEffect(() => {
    if (selectedOpponentPlayer !== allOption && !filterOptionExists(opponentPlayerOptions, selectedOpponentPlayer)) {
      setSelectedOpponentPlayer(allOption);
    }
  }, [opponentPlayerOptions, selectedOpponentPlayer]);

  useEffect(() => {
    if (selectedAlliance !== allOption && !filterOptionExists(allianceOptions, selectedAlliance)) {
      setSelectedAlliance(allOption);
    }
  }, [allianceOptions, selectedAlliance]);

  const filteredReports = useMemo(() => {
    const startMs = dateStartMs(startDate);
    const endMs = dateEndMs(endDate);
    const query = searchTerm.trim().toLowerCase();

    return reports
      .filter(hasBothPlayers)
      .filter((report) => {
        const reportTime = reportTimeMs(report);
        if (startMs !== null && reportTime !== null && reportTime < startMs) {
          return false;
        }
        if (endMs !== null && reportTime !== null && reportTime > endMs) {
          return false;
        }
        if (
          selectedResult !== allOption &&
          relativeOutcomeLabel(report, selectedPlayer, allianceMemberKeys, homeAllianceKey) !== selectedResult
        ) {
          return false;
        }
        if (selectedPlayer !== allOption && !reportIncludesPlayer(report, selectedPlayer)) {
          return false;
        }
        if (selectedPlayer === allOption && !reportIncludesAllianceLens(report, allianceMemberKeys, homeAllianceKey)) {
          return false;
        }
        if (
          selectedAlliance !== allOption &&
          !reportMatchesOpponentAlliance(report, selectedPlayer, selectedAlliance, allianceMemberKeys, homeAllianceKey)
        ) {
          return false;
        }
        if (
          selectedOpponentPlayer !== allOption &&
          !reportMatchesOpponentPlayer(report, selectedPlayer, selectedOpponentPlayer, allianceMemberKeys, homeAllianceKey)
        ) {
          return false;
        }
        if (selectedRole !== allOption && reportRole(report, selectedPlayer) !== selectedRole) {
          return false;
        }
        if (query && !searchBlob(report).includes(query)) {
          return false;
        }
        return true;
      })
      .sort((a, b) => (reportTimeMs(b) ?? 0) - (reportTimeMs(a) ?? 0));
  }, [reports, searchTerm, selectedAlliance, selectedOpponentPlayer, selectedPlayer, selectedResult, selectedRole, startDate, endDate, allianceMemberKeys, homeAllianceKey]);

  const summary = useMemo(
    () => summarizeReports(filteredReports, selectedPlayer, allianceMemberKeys, homeAllianceKey),
    [filteredReports, selectedPlayer, allianceMemberKeys, homeAllianceKey]
  );
  const detailReport = useMemo(() => {
    if (!selectedReportID) {
      return null;
    }

    return reports.find((report) => reportID(report) === selectedReportID) ?? null;
  }, [reports, selectedReportID]);
  const playerAggregates = useMemo(
    () => buildPlayerAggregates(filteredReports, selectedPlayer, allianceMemberKeys, homeAllianceKey),
    [filteredReports, selectedPlayer, allianceMemberKeys, homeAllianceKey]
  );
  const allianceAggregates = useMemo(
    () => buildAllianceAggregates(filteredReports, selectedPlayer, allianceMemberKeys, homeAllianceKey),
    [filteredReports, selectedPlayer, allianceMemberKeys, homeAllianceKey]
  );

  const resetFilters = () => {
    setSearchTerm('');
    setSelectedPlayer(allOption);
    setSelectedOpponentPlayer(allOption);
    setSelectedAlliance(allOption);
    setSelectedResult(allOption);
    setSelectedRole(allOption);
    setStartDate(inputDateFromDaysAgo(90));
    setEndDate(inputDateFromDate(new Date()));
  };

  if (detailReport) {
    return (
      <ReportDetailPage
        report={detailReport}
        outcome={relativeOutcomeLabel(detailReport, selectedPlayer, allianceMemberKeys, homeAllianceKey)}
        perspectiveSide={friendlySideForReport(detailReport, selectedPlayer, allianceMemberKeys, homeAllianceKey)}
        onBack={() => setSelectedReportID(null)}
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-4 xl:flex-row xl:items-start">
        <aside className="xl:w-80 shrink-0">
          <Card variant="solid">
            <CardHeader>
              <div>
                <CardTitle>Battle Stats</CardTitle>
                <p className="text-xs text-text-muted mt-1">{sourceLabel}</p>
              </div>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => void loadReports()}
                isLoading={isLoading}
                title="Refresh battle reports"
              >
                <RefreshCw className="w-4 h-4" />
              </Button>
            </CardHeader>
            <CardContent className="space-y-4">
              <Input
                value={searchTerm}
                onChange={(event) => setSearchTerm(event.target.value)}
                placeholder="Find player, alliance, castle"
                leftIcon={<Search className="w-4 h-4" />}
              />

              <FilterField label="Date range" icon={<CalendarDays className="w-4 h-4" />}>
                <div className="grid grid-cols-2 gap-2">
                  <Input type="date" value={startDate} onChange={(event) => setStartDate(event.target.value)} />
                  <Input type="date" value={endDate} onChange={(event) => setEndDate(event.target.value)} />
                </div>
              </FilterField>

              <FilterField label="Alliance player" icon={<Users className="w-4 h-4" />}>
                <Select value={selectedPlayer} options={playerOptions} onChange={setSelectedPlayer} menuGrowToViewport />
              </FilterField>

              <FilterField label="Opponent player" icon={<Swords className="w-4 h-4" />}>
                <Select
                  value={selectedOpponentPlayer}
                  options={opponentPlayerOptions}
                  onChange={setSelectedOpponentPlayer}
                  menuGrowToViewport
                />
              </FilterField>

              <FilterField label="Opponent alliance" icon={<Shield className="w-4 h-4" />}>
                <Select value={selectedAlliance} options={allianceOptions} onChange={setSelectedAlliance} menuGrowToViewport />
              </FilterField>

              <FilterField label="Result" icon={<BarChart3 className="w-4 h-4" />}>
                <Select
                  value={selectedResult}
                  onChange={setSelectedResult}
                  options={[
                    { value: allOption, label: 'All results' },
                    { value: 'Attack won', label: 'Attack won' },
                    { value: 'Attack lost', label: 'Attack lost' },
                    { value: 'Defense win', label: 'Defense win' },
                    { value: 'Defense lost', label: 'Defense lost' },
                  ]}
                />
              </FilterField>

              <FilterField label="Role" icon={<Castle className="w-4 h-4" />}>
                <Select
                  value={selectedRole}
                  onChange={setSelectedRole}
                  options={[
                    { value: allOption, label: 'All roles' },
                    { value: 'Attacker', label: 'Attacker' },
                    { value: 'Defender', label: 'Defender' },
                  ]}
                />
              </FilterField>

              <Button variant="outline" className="w-full" onClick={resetFilters}>
                Reset filters
              </Button>

              {loadError && (
                <div className="text-sm text-error bg-error/10 border border-error/20 rounded-global px-3 py-2">
                  {loadError}
                </div>
              )}
            </CardContent>
          </Card>
        </aside>

        <section className="flex-1 min-w-0 space-y-4">
          <div className="grid grid-cols-2 xl:grid-cols-6 gap-3">
            <StatCard label="Reports counted" value={formatNumber(summary.reports)} tone="neutral" />
            <StatCard label="Wins" value={formatNumber(summary.victories)} tone="success" />
            <StatCard label="Losses" value={formatNumber(summary.defeats)} tone="danger" />
            <StatCard label="Attack lost" value={formatNumber(summary.attackLost)} tone="danger" />
            <StatCard label="Defense lost" value={formatNumber(summary.defenseLost)} tone="info" />
            <StatCard label="Defenders killed" value={formatNumber(summary.defendersKilled)} tone="info" />
          </div>

          <div className="grid 2xl:grid-cols-2 gap-4">
            <PlayerAggregateTable rows={playerAggregates} />
            <AllianceAggregateTable rows={allianceAggregates} />
          </div>

          <Card variant="solid">
            <CardHeader>
              <div>
                <CardTitle>Reports</CardTitle>
                <p className="text-xs text-text-muted mt-1">
                  Showing {formatNumber(filteredReports.length)} of {formatNumber(reports.filter(hasBothPlayers).length)} parsed player reports
                </p>
              </div>
              <Badge variant="secondary">{isLoading ? 'Loading' : 'Ready'}</Badge>
            </CardHeader>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs uppercase tracking-wider text-text-muted border-b border-border-base">
                    <th className="px-4 py-3 font-semibold">Time</th>
                    <th className="px-4 py-3 font-semibold">Attacker</th>
                    <th className="px-4 py-3 font-semibold">Defender</th>
                    <th className="px-4 py-3 font-semibold">Result</th>
                    <th className="px-4 py-3 font-semibold">Castle</th>
                    <th className="px-4 py-3 font-semibold text-right">Attack lost</th>
                    <th className="px-4 py-3 font-semibold text-right">Def lost</th>
                    <th className="px-3 py-3 font-semibold text-right w-12" aria-label="Open details"></th>
                  </tr>
                </thead>
                <tbody>
                  {filteredReports.map((report) => (
                    <tr
                      key={reportID(report)}
                      className="border-b border-border-base/70 hover:bg-bg-card-hover transition-colors"
                    >
                      <td className="px-4 py-3 text-text-muted whitespace-nowrap">{formatDate(report)}</td>
                      <td className="px-4 py-3">
                        <CombatantCell combatant={report.attacker} />
                      </td>
                      <td className="px-4 py-3">
                        <CombatantCell combatant={report.defender} />
                      </td>
                      <td className="px-4 py-3">
                        <ReportResultBadges result={relativeOutcomeLabel(report, selectedPlayer, allianceMemberKeys, homeAllianceKey)} />
                      </td>
                      <td className="px-4 py-3 text-text-main whitespace-nowrap">{battleLocationLabel(report)}</td>
                      <td className="px-4 py-3 text-right text-error font-semibold">
                        {formatNumber(metricValue(report.metrics, 'attackerLost', 'attackLost'))}
                      </td>
                      <td className="px-4 py-3 text-right text-info font-semibold">
                        {formatNumber(metricValue(report.metrics, 'defenderLost', 'defenseLost'))}
                      </td>
                      <td className="px-3 py-3 text-right">
                        <Button
                          variant="secondary"
                          size="icon"
                          className="h-9 w-9 border-primary/40 text-primary hover:border-primary hover:bg-primary/10"
                          onClick={() => setSelectedReportID(reportID(report))}
                          title="Go to report details"
                          aria-label={`Go to details for ${combatantName(report.attacker)} vs ${combatantName(report.defender)}`}
                        >
                          <ArrowRight className="w-4 h-4" />
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {filteredReports.length === 0 && (
              <div className="px-5 py-12 text-center text-text-muted">
                No player battle reports match the current filters.
              </div>
            )}
          </Card>
        </section>
      </div>
    </div>
  );
};

interface FilterFieldProps {
  label: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}

const FilterField: React.FC<FilterFieldProps> = ({ label, icon, children }) => (
  <div className="space-y-2">
    <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-text-muted">
      {icon}
      <span>{label}</span>
    </div>
    {children}
  </div>
);

interface StatCardProps {
  label: string;
  value: string;
  tone: 'neutral' | 'success' | 'danger' | 'info';
}

const StatCard: React.FC<StatCardProps> = ({ label, value, tone }) => {
  const toneClass = {
    neutral: 'text-text-main',
    success: 'text-success',
    danger: 'text-error',
    info: 'text-info',
  }[tone];

  return (
    <Card variant="solid" className="p-4">
      <div className="text-xs uppercase tracking-wider text-text-muted font-semibold">{label}</div>
      <div className={`text-2xl font-bold mt-2 ${toneClass}`}>{value}</div>
    </Card>
  );
};

const CombatantCell: React.FC<{ combatant?: BattleCombatant }> = ({ combatant }) => (
  <div className="min-w-40">
    <div className="font-semibold text-text-main">{combatantName(combatant)}</div>
    <div className="text-xs text-text-muted">{allianceName(combatant)}</div>
  </div>
);

const ReportResultBadges: React.FC<{ result: string; size?: 'sm' | 'lg'; className?: string }> = ({
  result,
  size = 'sm',
  className = '',
}) => {
  const parts = splitResultLabel(result);
  if (!parts) {
    return <Badge variant="secondary">Unknown</Badge>;
  }

  const roleClass =
    parts.role === 'Attack'
      ? 'border-warning/30 bg-warning/10 text-warning'
      : 'border-info/30 bg-info/10 text-info';
  const layoutClass = size === 'lg' ? 'justify-center gap-2' : 'min-w-[8rem] gap-1.5';
  const badgeClass = size === 'lg' ? 'px-4 py-1.5 text-sm md:text-base' : '';

  return (
    <div className={`flex flex-wrap items-center ${layoutClass} ${className}`}>
      <Badge variant="outline" className={`${roleClass} ${badgeClass}`}>{parts.role}</Badge>
      <Badge variant={parts.won ? 'success' : 'danger'} className={badgeClass}>{parts.won ? 'Won' : 'Lost'}</Badge>
    </div>
  );
};

const ResultBadge: React.FC<{ result: string }> = ({ result }) => {
  if (result === 'Attack won' || result === 'Defense win') {
    return <Badge variant="success">{result}</Badge>;
  }
  if (result === 'Attack lost' || result === 'Defense lost') {
    return <Badge variant="danger">{result}</Badge>;
  }
  return <Badge variant="secondary">Unknown</Badge>;
};

interface PlayerAggregate {
  key: string;
  name: string;
  alliance: string;
  reports: number;
  attacks: number;
  defenses: number;
  wins: number;
  losses: number;
  unitsLost: number;
  unitsKilled: number;
  attackDefendersKilled: number;
  attackAttackersLost: number;
  defenseDefendersLost: number;
  defenseAttackersKilled: number;
}

interface AllianceAggregate {
  key: string;
  name: string;
  reports: number;
  wins: number;
  losses: number;
  unitsLost: number;
  unitsKilled: number;
  attackDefendersKilled: number;
  attackAttackersLost: number;
  defenseDefendersLost: number;
  defenseAttackersKilled: number;
}

const PlayerAggregateTable: React.FC<{ rows: PlayerAggregate[] }> = ({ rows }) => (
  <Card variant="solid">
    <CardHeader>
      <div>
        <CardTitle>Player Aggregate</CardTitle>
        <p className="text-xs text-text-muted mt-1">Filtered battle totals by player</p>
      </div>
      <Badge variant="secondary">{rows.length} players</Badge>
    </CardHeader>
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs uppercase tracking-wider text-text-muted border-b border-border-base">
            <th className="px-4 py-3 font-semibold">Player</th>
            <th className="px-4 py-3 font-semibold text-right">Reports</th>
            <th className="px-4 py-3 font-semibold text-right">A / D</th>
            <th className="px-4 py-3 font-semibold text-right">W / L</th>
            <th className="px-4 py-3 font-semibold text-right" title="Defenders killed per attacker lost in attacks by this player">Attack Ratio</th>
            <th className="px-4 py-3 font-semibold text-right" title="Defenders lost per attacker killed in defenses by this player">Defense Ratio</th>
          </tr>
        </thead>
        <tbody>
          {rows.slice(0, 10).map((row) => (
            <tr key={row.key} className="border-b border-border-base/70">
              <td className="px-4 py-3">
                <div className="font-semibold text-text-main">{row.name}</div>
                <div className="text-xs text-text-muted">{row.alliance}</div>
              </td>
              <td className="px-4 py-3 text-right text-text-main font-semibold">{formatNumber(row.reports)}</td>
              <td className="px-4 py-3 text-right text-text-muted">{formatNumber(row.attacks)} / {formatNumber(row.defenses)}</td>
              <td className="px-4 py-3 text-right">
                <span className="text-success font-semibold">{formatNumber(row.wins)}</span>
                <span className="text-text-muted"> / </span>
                <span className="text-error font-semibold">{formatNumber(row.losses)}</span>
              </td>
              <td className="px-4 py-3 text-right text-info font-semibold">
                {formatRatio(row.attackDefendersKilled, row.attackAttackersLost)}
              </td>
              <td className="px-4 py-3 text-right text-error font-semibold">
                {formatRatio(row.defenseDefendersLost, row.defenseAttackersKilled)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
    {rows.length === 0 && (
      <div className="px-5 py-10 text-center text-text-muted">No player aggregate for the current filters.</div>
    )}
  </Card>
);

const AllianceAggregateTable: React.FC<{ rows: AllianceAggregate[] }> = ({ rows }) => (
  <Card variant="solid">
    <CardHeader>
      <div>
        <CardTitle>Alliance Aggregate</CardTitle>
        <p className="text-xs text-text-muted mt-1">Filtered battle totals by alliance</p>
      </div>
      <Badge variant="secondary">{rows.length} alliances</Badge>
    </CardHeader>
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs uppercase tracking-wider text-text-muted border-b border-border-base">
            <th className="px-4 py-3 font-semibold">Alliance</th>
            <th className="px-4 py-3 font-semibold text-right">Reports</th>
            <th className="px-4 py-3 font-semibold text-right">W / L</th>
            <th className="px-4 py-3 font-semibold text-right" title="Defenders killed per attacker lost when attacking this alliance">Attack Ratio</th>
            <th className="px-4 py-3 font-semibold text-right" title="Defenders lost per attacker killed when defending against this alliance">Defense Ratio</th>
          </tr>
        </thead>
        <tbody>
          {rows.slice(0, 10).map((row) => (
            <tr key={row.key} className="border-b border-border-base/70">
              <td className="px-4 py-3 font-semibold text-text-main">{row.name}</td>
              <td className="px-4 py-3 text-right text-text-main font-semibold">{formatNumber(row.reports)}</td>
              <td className="px-4 py-3 text-right">
                <span className="text-success font-semibold">{formatNumber(row.wins)}</span>
                <span className="text-text-muted"> / </span>
                <span className="text-error font-semibold">{formatNumber(row.losses)}</span>
              </td>
              <td className="px-4 py-3 text-right text-info font-semibold">
                {formatRatio(row.attackDefendersKilled, row.attackAttackersLost)}
              </td>
              <td className="px-4 py-3 text-right text-error font-semibold">
                {formatRatio(row.defenseDefendersLost, row.defenseAttackersKilled)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
    {rows.length === 0 && (
      <div className="px-5 py-10 text-center text-text-muted">No alliance aggregate for the current filters.</div>
    )}
  </Card>
);

const ReportDetailPage: React.FC<{
  report: ParsedReport;
  outcome: string;
  perspectiveSide: CombatantSide | '';
  onBack: () => void;
}> = ({
  report,
  outcome,
  perspectiveSide,
  onBack,
}) => (
  <div className="space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div>
        <div className="text-xs uppercase tracking-wider text-text-muted font-semibold">Battle report</div>
        <h2 className="text-2xl font-bold text-text-main mt-1">{battleLocationLabel(report)}</h2>
      </div>
      <Button variant="outline" onClick={onBack}>
        Back to aggregate
      </Button>
    </div>
    <ReportDetails report={report} outcome={outcome} perspectiveSide={perspectiveSide} />
  </div>
);

const ReportDetails: React.FC<{ report: ParsedReport; outcome: string; perspectiveSide: CombatantSide | '' }> = ({
  report,
  outcome,
  perspectiveSide,
}) => {
  const commanderEffects = effectsForSide(report, 'commander');
  const castellanEffects = effectsForSide(report, 'castellan');
  const attackerUnits = itemsForSide(report.topUnits, 'attacker').slice(0, 12);
  const defenderUnits = itemsForSide(report.topUnits, 'defender').slice(0, 12);
  const attackerTools = itemsForSide(report.supportTools, 'attacker').slice(0, 12);
  const defenderTools = itemsForSide(report.supportTools, 'defender').slice(0, 12);

  return (
    <div className="space-y-4">
      <BattleStatusBanner report={report} outcome={outcome} />

      <UnitStatsPanel report={report} perspectiveSide={perspectiveSide} />

      <div className="grid xl:grid-cols-2 gap-4">
        <ForcePanel
          title="Attacker forces"
          combatant={report.attacker}
          units={attackerUnits}
          tools={attackerTools}
          tone="danger"
        />
        <ForcePanel
          title="Defender forces"
          combatant={report.defender}
          units={defenderUnits}
          tools={defenderTools}
          tone="info"
        />
      </div>

      <EffectComparison
        commanderName={combatantName(report.attacker)}
        commanderEffects={commanderEffects}
        castellanName={combatantName(report.defender)}
        castellanEffects={castellanEffects}
      />

      {report.waves && report.waves.length > 0 && (
        <Card variant="solid">
          <CardHeader>
            <CardTitle>Wall Waves</CardTitle>
            <Badge variant="secondary">{report.waves.length} waves</Badge>
          </CardHeader>
          <CardContent className="space-y-3">
            {report.waves.map((wave, index) => (
              <WaveRow key={`${wave.wave ?? wave.index ?? index}-${index}`} wave={wave} index={index} />
            ))}
          </CardContent>
        </Card>
      )}
    </div>
  );
};

const BattleStatusBanner: React.FC<{ report: ParsedReport; outcome: string }> = ({ report, outcome }) => {
  return (
    <Card variant="solid" className="overflow-hidden">
      <CardContent className="!p-0">
        <div className="relative overflow-hidden bg-gradient-to-r from-error/10 via-bg-card to-info/10 p-5">
          <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(18rem,0.8fr)_minmax(0,1fr)] xl:items-stretch">
            <BattleBannerSide label="Attacker" combatant={report.attacker} tone="danger" />

            <div className="flex min-w-0 flex-col items-center justify-center rounded-global border border-border-base bg-bg-card/90 px-5 py-4 text-center shadow-sm">
              <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-global bg-primary/10 text-primary ring-1 ring-primary/20">
                <Swords className="h-6 w-6" />
              </div>
              <ReportResultBadges result={outcome} size="lg" />
              <div className="mt-3 text-sm font-semibold text-text-main">{battleLocationLabel(report)}</div>
            </div>

            <BattleBannerSide label="Defender" combatant={report.defender} tone="info" align="right" />
          </div>

          <div className="mt-4 grid gap-2 md:grid-cols-3">
            <BannerFact icon={<CalendarDays className="h-4 w-4" />} label="Date" value={formatDate(report)} />
            <BannerFact icon={<MapPin className="h-4 w-4" />} label="Coordinates" value={battleCoordinateLabel(report)} />
            <BannerFact icon={<Shield className="h-4 w-4" />} label="Kingdom" value={kingdomLabel(report)} />
          </div>
        </div>
      </CardContent>
    </Card>
  );
};

const BattleBannerSide: React.FC<{
  label: string;
  combatant?: BattleCombatant;
  tone: 'danger' | 'info';
  align?: 'left' | 'right';
}> = ({ label, combatant, tone, align = 'left' }) => {
  const toneClass = tone === 'danger' ? 'border-error/30 text-error' : 'border-info/30 text-info';
  const alignClass = align === 'right' ? 'items-start text-left xl:items-end xl:text-right' : 'items-start text-left';

  return (
    <div className={`flex min-w-0 flex-col justify-center rounded-global border bg-bg-card/80 p-4 shadow-sm ${toneClass} ${alignClass}`}>
      <div className="text-xs uppercase tracking-wider text-text-muted font-semibold">{label}</div>
      <div className="mt-2 max-w-full truncate text-2xl font-black text-text-main">{combatantName(combatant)}</div>
      <div className="mt-1 max-w-full truncate text-sm font-medium text-text-muted">{allianceName(combatant)}</div>
      {combatant?.castleName && (
        <div className="mt-3 max-w-full truncate rounded-full bg-bg-app px-3 py-1 text-xs font-semibold text-text-main ring-1 ring-border-base">
          {combatant.castleName}
        </div>
      )}
    </div>
  );
};

const BannerFact: React.FC<{ icon: React.ReactNode; label: string; value: string }> = ({ icon, label, value }) => (
  <div className="flex min-w-0 items-center gap-3 rounded-global border border-border-base bg-bg-card/80 px-3 py-2 shadow-sm">
    <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">{icon}</span>
    <div className="min-w-0">
      <div className="text-[10px] uppercase tracking-wider text-text-muted font-semibold">{label}</div>
      <div className="truncate text-sm font-semibold text-text-main">{value}</div>
    </div>
  </div>
);

const UnitStatsPanel: React.FC<{ report: ParsedReport; perspectiveSide: CombatantSide | '' }> = ({
  report,
  perspectiveSide,
}) => {
  const side = perspectiveSide || 'attacker';
  const attackerSent = metricValue(report.metrics, 'attackerSent', 'attackSent');
  const attackerLost = metricValue(report.metrics, 'attackerLost', 'attackLost');
  const defenderStationed = metricValue(report.metrics, 'defenderStationed');
  const defenderLost = metricValue(report.metrics, 'defenderLost', 'defenseLost');
  const isDefenseView = side === 'defender';
  const ourForce = isDefenseView ? defenderStationed : attackerSent;
  const ourLost = isDefenseView ? defenderLost : attackerLost;
  const opponentForce = isDefenseView ? attackerSent : defenderStationed;
  const opponentLost = isDefenseView ? attackerLost : defenderLost;
  const totalLosses = attackerLost + defenderLost;
  const tradeRatio = formatTradeRatio(opponentLost, ourLost);
  const tradeTone = opponentLost > ourLost ? 'success' : opponentLost < ourLost ? 'danger' : 'neutral';

  return (
    <Card variant="solid" className="overflow-hidden">
      <CardHeader className="bg-gradient-to-r from-error/5 via-bg-card to-info/5">
        <div>
          <CardTitle>All Unit Stats</CardTitle>
          <p className="text-xs text-text-muted mt-1">
            {isDefenseView ? 'Defense perspective' : 'Attack perspective'} based on the selected player or alliance.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant={isDefenseView ? 'primary' : 'danger'}>{isDefenseView ? 'Defender view' : 'Attacker view'}</Badge>
          <BarChart3 className="w-5 h-5 text-primary" />
        </div>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
          <MetricTile
            label={isDefenseView ? 'Our stationed' : 'Our sent'}
            value={ourForce}
            tone={isDefenseView ? 'info' : 'neutral'}
          />
          <MetricTile label="Our losses" value={ourLost} tone="danger" />
          <MetricTile
            label={isDefenseView ? 'Opponent sent' : 'Opponent stationed'}
            value={opponentForce}
            tone={isDefenseView ? 'neutral' : 'info'}
          />
          <MetricTile label="Opponent losses" value={opponentLost} tone="success" />
          <MetricTile label="Trade ratio" value={tradeRatio} tone={tradeTone} caption="Opponent losses per our loss" />
          <MetricTile label="Total losses" value={totalLosses} tone="danger" caption="Both sides combined" />
        </div>
      </CardContent>
    </Card>
  );
};

const MetricTile: React.FC<{
  label: string;
  value: number | string;
  tone?: 'neutral' | 'success' | 'danger' | 'info';
  caption?: string;
}> = ({
  label,
  value,
  tone = 'neutral',
  caption,
}) => {
  const toneClass = {
    neutral: 'text-text-main',
    success: 'text-success',
    danger: 'text-error',
    info: 'text-info',
  }[tone];
  const borderClass = {
    neutral: 'border-border-base',
    success: 'border-success/20',
    danger: 'border-error/20',
    info: 'border-info/20',
  }[tone];
  const displayValue = typeof value === 'number' ? formatNumber(value) : value;

  return (
    <div className={`border rounded-global px-3 py-3 bg-bg-app ${borderClass}`}>
      <div className="text-xs text-text-muted">{label}</div>
      <div className={`text-lg font-bold mt-1 ${toneClass}`}>{displayValue}</div>
      {caption && <div className="mt-1 text-[11px] text-text-muted">{caption}</div>}
    </div>
  );
};

const EffectComparison: React.FC<{
  commanderName: string;
  commanderEffects: BattleEffect[];
  castellanName: string;
  castellanEffects: BattleEffect[];
}> = ({ commanderName, commanderEffects, castellanName, castellanEffects }) => {
  const groups = effectComparisonGroups(commanderEffects, castellanEffects);
  const totalEffects = commanderEffects.length + castellanEffects.length;

  return (
    <Card variant="solid">
      <CardHeader>
        <div>
          <CardTitle>Commander / Castellan</CardTitle>
        </div>
        <Badge variant="secondary">{totalEffects} effects</Badge>
      </CardHeader>
      <CardContent>
        {totalEffects > 0 ? (
          <div className="overflow-x-auto">
            <div className="min-w-[44rem] space-y-4">
              <div className="grid grid-cols-2 gap-3">
                <EffectComparisonHeader label="Commander" name={commanderName} tone="danger" />
                <EffectComparisonHeader label="Castellan" name={castellanName} tone="info" align="right" />
              </div>

              {groups.map((group) => (
                <div key={group.category} className="space-y-2">
                  <div className="text-[11px] uppercase tracking-wider font-bold text-text-muted">
                    {group.category}
                  </div>
                  <div className="overflow-hidden rounded-global border border-border-base bg-bg-app">
                    {group.rows.map((row) => (
                      <div
                        key={row.key}
                        className="grid grid-cols-2 divide-x divide-border-base border-b border-border-base last:border-b-0"
                      >
                        <EffectComparisonCell effect={row.commander} side="commander" />
                        <EffectComparisonCell effect={row.castellan} side="castellan" />
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <div className="text-sm text-text-muted py-8 text-center">No parsed leader effects for this report.</div>
        )}
      </CardContent>
    </Card>
  );
};

const EffectComparisonHeader: React.FC<{
  label: string;
  name: string;
  tone: 'danger' | 'info';
  align?: 'left' | 'right';
}> = ({ label, name, tone, align = 'left' }) => {
  const toneClass = tone === 'danger' ? 'border-error/25 text-error' : 'border-info/25 text-info';
  const alignClass = align === 'right' ? 'items-end text-right' : 'items-start text-left';

  return (
    <div className={`flex min-w-0 flex-col rounded-global border bg-bg-app px-3 py-2 ${toneClass} ${alignClass}`}>
      <div className="text-[10px] font-bold uppercase tracking-wider">{label}</div>
      <div className="mt-1 max-w-full truncate text-sm font-semibold text-text-main">{name}</div>
    </div>
  );
};

const EffectComparisonCell: React.FC<{ effect?: BattleEffect; side: 'commander' | 'castellan' }> = ({ effect, side }) => {
  if (!effect) {
    return <div className="min-h-12 px-3 py-2.5 text-xs font-semibold text-text-muted/50">-</div>;
  }

  const value = (
    <span className="shrink-0 rounded-full border border-success/25 bg-success/10 px-2.5 py-1 text-xs font-black tabular-nums text-success">
      {effectValue(effect)}
    </span>
  );
  const description = (
    <span className={`min-w-0 text-sm font-medium text-text-main ${side === 'castellan' ? 'text-right' : ''}`}>
      {effectDescription(effect)}
    </span>
  );

  return (
    <div className="flex min-h-12 items-start justify-between gap-3 px-3 py-2.5">
      {side === 'commander' ? (
        <>
          {description}
          {value}
        </>
      ) : (
        <>
          {value}
          {description}
        </>
      )}
    </div>
  );
};

const ForcePanel: React.FC<{
  title: string;
  combatant?: BattleCombatant;
  units: BattleItemDetail[];
  tools: BattleItemDetail[];
  tone: 'danger' | 'info';
}> = ({ title, combatant, units, tools, tone }) => {
  const accentClass = tone === 'danger' ? 'text-error' : 'text-info';

  return (
    <Card variant="solid">
      <CardHeader>
        <div>
          <CardTitle>{title}</CardTitle>
          <p className="text-xs text-text-muted mt-1">{combatantName(combatant)}</p>
        </div>
        <Badge variant="secondary">{units.length + tools.length} parsed</Badge>
      </CardHeader>
      <CardContent className="space-y-4">
        <RosterSection title="Units fought" items={units} kind="unit" valueClass={accentClass} />
        <RosterSection title="Tools used" items={tools} kind="tool" valueClass={accentClass} />
      </CardContent>
    </Card>
  );
};

const RosterSection: React.FC<{
  title: string;
  items: BattleItemDetail[];
  kind: 'unit' | 'tool';
  valueClass: string;
}> = ({ title, items, kind, valueClass }) => (
  <div>
    <div className="flex items-center justify-between gap-2 mb-2">
      <div className="text-xs uppercase tracking-wider text-text-muted font-semibold">{title}</div>
      <span className="text-xs text-text-muted">{items.length}</span>
    </div>
    {items.length > 0 ? (
      <div className="grid grid-cols-[repeat(auto-fill,minmax(6.5rem,6.5rem))] gap-3">
        {items.map((item, index) => (
          <BattleItemChip key={`${kind}-${itemKey(item)}-${index}`} item={item} kind={kind} valueClass={valueClass} />
        ))}
      </div>
    ) : (
      <div className="border border-border-base rounded-global bg-bg-app px-3 py-4 text-sm text-text-muted text-center">
        No parsed {kind === 'unit' ? 'units' : 'tools'}.
      </div>
    )}
  </div>
);

const BattleItemChip: React.FC<{
  item: BattleItemDetail;
  kind: 'unit' | 'tool';
  valueClass: string;
  compact?: boolean;
}> = ({ item, kind, valueClass, compact = false }) => {
  const { getTroop, getTool } = useMetadata();
  const id = itemID(item);
  const metadata = kind === 'unit' ? getTroop(id) : getTool(id);
  const name = metadata?.name ?? `${kind === 'unit' ? 'Unit' : 'Tool'} ${id}`;
  const amount = numericValue(item.amount) ?? 0;
  const delta = kind === 'unit' ? numericValue(item.lost) ?? 0 : numericValue(item.used) ?? 0;
  const changeValue = kind === 'unit' ? delta : delta || amount;
  const size = compact ? 52 : 68;
  const isDangerTone = valueClass.includes('error');
  const cardSizeClass = compact ? 'h-[7.25rem] w-20 p-1.5' : 'h-36 w-[6.5rem] p-2';
  const iconAreaClass = compact ? 'h-[52px]' : 'h-[68px]';
  const cardToneClass = isDangerTone
    ? 'border-error/20 hover:border-error/45'
    : 'border-info/20 hover:border-info/45';
  const deltaClass = isDangerTone
    ? 'border-error/20 bg-error/10 text-error'
    : 'border-info/20 bg-info/10 text-info';
  const phase = phaseLabel(item.phase);
  const title = [
    name,
    `x${formatNumber(amount)}`,
    phase,
    changeValue > 0 ? `${kind === 'unit' ? 'lost' : 'used'} ${formatNumber(changeValue)}` : '',
  ]
    .filter(Boolean)
    .join(' · ');

  return (
    <div
      className={`group relative flex shrink-0 flex-col items-center justify-between overflow-hidden rounded-global border bg-bg-card/90 shadow-sm ring-1 ring-border-base/40 transition-all duration-200 hover:bg-bg-card-hover hover:shadow-glow ${cardSizeClass} ${cardToneClass}`}
      title={title}
      aria-label={title}
    >
      <div className={`relative flex w-full items-center justify-center ${iconAreaClass}`}>
        {kind === 'unit' ? (
          <UnitImage unitId={id} size={size} showLevel className="!bg-transparent drop-shadow-md" />
        ) : (
          <ToolImage toolId={id} size={size} className="!bg-transparent drop-shadow-md" />
        )}
      </div>
      <div className="flex w-full shrink-0 flex-col items-center gap-1">
        <span className="max-w-full rounded-full border border-border-base/70 bg-bg-app px-2 py-1 text-center text-sm font-bold leading-none text-text-main shadow-sm tabular-nums">
          {formatNumber(amount)}
        </span>
        {changeValue > 0 && (
          <span className={`max-w-full rounded-full border px-2 py-0.5 text-center text-[11px] font-bold leading-none tabular-nums ${deltaClass}`}>
            -{formatNumber(changeValue)}
          </span>
        )}
      </div>
    </div>
  );
};

const WaveRow: React.FC<{ wave: BattleWave; index: number }> = ({ wave, index }) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const lanes = wave.lanes ?? [];
  const waveLabel = `Wave ${wave.wave ?? wave.index ?? index + 1}`;
  const detailsId = `wave-details-${wave.wave ?? wave.index ?? index}-${index}`;

  return (
    <div className="border border-border-base rounded-global bg-bg-app p-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <button
          type="button"
          className="flex min-w-0 items-center gap-2 text-left font-semibold text-text-main transition-colors hover:text-primary"
          aria-expanded={isExpanded}
          aria-controls={detailsId}
          onClick={() => setIsExpanded((current) => !current)}
        >
          {isExpanded ? (
            <ChevronDown className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
          ) : (
            <ChevronRight className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
          )}
          <span>{waveLabel}</span>
        </button>
        {lanes.length > 0 && <WaveLaneSummary lanes={lanes} />}
      </div>
      {isExpanded && lanes.length > 0 && (
        <div id={detailsId} className="mt-3 grid xl:grid-cols-3 gap-3">
          {lanes.map((lane, laneIndex) => (
            <LaneDetailCard key={`${lane.lane ?? laneIndex}-${laneIndex}`} lane={lane} laneIndex={laneIndex} />
          ))}
        </div>
      )}
      {lanes.length === 0 && (
        <div className="text-sm text-text-muted">No lane details parsed for this wave.</div>
      )}
    </div>
  );
};

const WaveLaneSummary: React.FC<{ lanes: BattleWaveLane[] }> = ({ lanes }) => (
  <div className="flex flex-wrap items-center gap-x-3 gap-y-2 sm:justify-end">
    {lanes.map((lane, laneIndex) => {
      const result = laneResult(lane);

      return (
        <div key={`${lane.lane ?? laneIndex}-${laneIndex}`} className="flex items-center gap-1.5">
          <span className="text-[11px] uppercase tracking-wider text-text-muted font-semibold">
            {laneLabel(lane, laneIndex)}
          </span>
          <Badge variant={result === 'HELD' ? 'success' : 'warning'}>{result}</Badge>
        </div>
      );
    })}
  </div>
);

const LaneDetailCard: React.FC<{ lane: BattleWaveLane; laneIndex: number }> = ({ lane, laneIndex }) => {
  const result = laneResult(lane);
  const attackerUnits = lane.attackerUnitDetails ?? [];
  const defenderUnits = lane.defenderUnitDetails ?? [];
  const attackerTools = lane.attackerToolDetails ?? [];
  const defenderTools = lane.defenderToolDetails ?? [];

  return (
    <div className="border border-border-base rounded-global bg-bg-card/40 p-3 min-w-0">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs uppercase tracking-wider text-text-muted font-semibold">
          {laneLabel(lane, laneIndex)}
        </span>
        <Badge variant={result === 'HELD' ? 'success' : 'warning'}>{result}</Badge>
      </div>
      <div className="mt-3 grid grid-cols-2 gap-4">
        <LaneSidePanel
          title="Attacker"
          lost={lane.attackerLost ?? 0}
          units={attackerUnits}
          tools={attackerTools}
          unitTitle="Units lost"
          toolTitle="Tools used"
          valueClass="text-error"
        />
        <LaneSidePanel
          title="Defender"
          lost={lane.defenderLost ?? 0}
          units={defenderUnits}
          tools={defenderTools}
          unitTitle="Units lost"
          toolTitle="Tools used"
          valueClass="text-info"
        />
      </div>
    </div>
  );
};

const LaneSidePanel: React.FC<{
  title: string;
  lost: number;
  units: BattleItemDetail[];
  tools: BattleItemDetail[];
  unitTitle: string;
  toolTitle: string;
  valueClass: string;
}> = ({ title, lost, units, tools, unitTitle, toolTitle, valueClass }) => {
  return (
    <div className="min-w-0">
      <div className="flex items-center justify-between gap-2">
        <span className="text-[11px] uppercase tracking-wider text-text-muted font-semibold">{title}</span>
        <span className={`rounded-full bg-bg-card px-2 py-0.5 text-xs font-bold tabular-nums shadow-sm ring-1 ring-border-base/70 ${valueClass}`}>
          {formatNumber(lost)}
        </span>
      </div>
      <div className="mt-2 space-y-2">
        <LaneItemStrip title={unitTitle} items={units} kind="unit" valueClass={valueClass} />
        <LaneItemStrip title={toolTitle} items={tools} kind="tool" valueClass={valueClass} />
      </div>
    </div>
  );
};

const LaneItemStrip: React.FC<{
  title: string;
  items: BattleItemDetail[];
  kind: 'unit' | 'tool';
  valueClass: string;
}> = ({ title, items, kind, valueClass }) => {
  if (items.length === 0) {
    return null;
  }

  return (
    <div>
      <div className="text-[11px] uppercase tracking-wider text-text-muted font-semibold mb-1.5">{title}</div>
      <div className="grid grid-cols-[repeat(auto-fill,minmax(5rem,5rem))] gap-2">
        {items.map((item, index) => (
          <BattleItemChip key={`${title}-${itemKey(item)}-${index}`} item={item} kind={kind} valueClass={valueClass} compact />
        ))}
      </div>
    </div>
  );
};

async function fetchReports(source: string): Promise<{ source: string; reports: ParsedReport[] } | null> {
  try {
    const response = await fetch(source, { cache: 'no-store' });
    if (!response.ok) {
      return null;
    }

    const text = await response.text();
    const reports = parseReportText(text);
    return { source, reports };
  } catch {
    return null;
  }
}

function parseReportText(text: string): ParsedReport[] {
  const trimmed = text.trim();
  if (!trimmed) {
    return [];
  }

  try {
    return reportsFromUnknown(JSON.parse(trimmed));
  } catch {
    return trimmed
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter(Boolean)
      .flatMap((line) => {
        try {
          return reportsFromUnknown(JSON.parse(line));
        } catch {
          return [];
        }
      });
  }
}

function reportsFromUnknown(value: unknown): ParsedReport[] {
  if (Array.isArray(value)) {
    return value.flatMap(reportsFromUnknown);
  }

  if (!isRecord(value)) {
    return [];
  }

  const nestedReports = value.reports ?? value.data ?? value.items;
  if (Array.isArray(nestedReports)) {
    return nestedReports.flatMap(reportsFromUnknown);
  }

  const parsed = value.parsed ?? value.parsedReport ?? value.report;
  if (isRecord(parsed)) {
    return [parsed as ParsedReport];
  }

  return [value as ParsedReport];
}

function hasBothPlayers(report: ParsedReport): boolean {
  return Boolean(combatantKey(report.attacker) && combatantKey(report.defender));
}

function summarizeReports(
  list: ParsedReport[],
  selectedPlayerKey: string,
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): BattleStatsSummary {
  return list.reduce<BattleStatsSummary>(
    (summary, report) => {
      const outcome = relativeOutcomeLabel(report, selectedPlayerKey, allianceMemberKeys, homeAllianceKey);
      summary.reports += 1;
      summary.victories += outcome === 'Attack won' || outcome === 'Defense win' ? 1 : 0;
      summary.defeats += outcome === 'Attack lost' || outcome === 'Defense lost' ? 1 : 0;
      summary.attackLost += metricValue(report.metrics, 'attackerLost', 'attackLost');
      summary.defenseLost += metricValue(report.metrics, 'defenderLost', 'defenseLost');
      summary.attackersKilled += metricValue(report.metrics, 'attackersKilled');
      summary.defendersKilled += metricValue(report.metrics, 'defendersKilled');
      return summary;
    },
    {
      reports: 0,
      victories: 0,
      defeats: 0,
      attackLost: 0,
      defenseLost: 0,
      attackersKilled: 0,
      defendersKilled: 0,
    }
  );
}

function allianceMemberOptionsFromSources(
  snapshot: Record<string, unknown> | null,
  allianceInfo: Record<string, unknown> | null
): AllianceMemberOption[] {
  const rows = new Map<string, AllianceMemberOption>();
  const gameState = isRecord(snapshot?.gameState) ? snapshot.gameState : null;
  addAllianceMemberOptions(rows, isRecord(gameState?.alliance) ? gameState.alliance : null);
  addAllianceMemberOptions(rows, allianceInfo);

  return Array.from(rows.values()).sort((a, b) => a.label.localeCompare(b.label));
}

function addAllianceMemberOptions(
  rows: Map<string, AllianceMemberOption>,
  alliance: Record<string, unknown> | null
) {
  const members = Array.isArray(alliance?.members) ? alliance.members : [];

  members.forEach((member) => {
    if (!isRecord(member)) {
      return;
    }

    const id = stringValue(member.playerID ?? member.playerId ?? member.oid ?? member.OID ?? member.id ?? member.ID);
    const name = stringValue(member.name ?? member.playerName ?? member.N ?? member.PN ?? member.Name);
    const value = id || name.toLowerCase();
    if (!value) {
      return;
    }

    const keys = new Set<string>();
    if (id) {
      keys.add(id);
    }
    if (name) {
      keys.add(name.toLowerCase());
    }

    rows.set(value, {
      value,
      label: name || `Player ${id}`,
      keys: Array.from(keys),
    });
  });
}

function allianceMemberKeysFromOptions(options: AllianceMemberOption[]): Set<string> {
  const keys = new Set<string>();

  options.forEach((option) => {
    option.keys.forEach((key) => keys.add(key));
  });

  return keys;
}

function buildAlliancePlayerOptions(
  reports: ParsedReport[],
  allianceMembers: AllianceMemberOption[],
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): FilterOption[] {
  if (allianceMembers.length > 0) {
    return [
      { value: allOption, label: 'All alliance players' },
      ...allianceMembers.map(({ value, label }) => ({ value, label })),
    ];
  }

  const options = new Map<string, string>();
  reports.forEach((report) => {
    [report.attacker, report.defender].forEach((combatant) => {
      const key = combatantKey(combatant);
      if (key && combatantInAllianceLens(combatant, allianceMemberKeys, homeAllianceKey)) {
        options.set(key, combatantName(combatant));
      }
    });
  });

  return [
    { value: allOption, label: 'All alliance players' },
    ...Array.from(options.entries())
      .sort((a, b) => a[1].localeCompare(b[1]))
      .map(([value, label]) => ({ value, label })),
  ];
}

function filterOptionExists(options: FilterOption[], value: string): boolean {
  return options.some((option) => option.value === value);
}

function resolveHomeAllianceKey(reports: ParsedReport[], allianceMemberKeys: Set<string>): string {
  const counts = new Map<string, number>();

  reports.forEach((report) => {
    [report.attacker, report.defender].forEach((combatant) => {
      if (!combatant || !combatantMatchesMemberKey(combatant, allianceMemberKeys)) {
        return;
      }
      const key = allianceKey(combatant);
      if (key) {
        counts.set(key, (counts.get(key) ?? 0) + 1);
      }
    });
  });

  if (counts.size === 0) {
    reports.forEach((report) => {
      [report.attacker, report.defender].forEach((combatant) => {
        const key = allianceKey(combatant);
        if (key) {
          counts.set(key, (counts.get(key) ?? 0) + 1);
        }
      });
    });
  }

  return Array.from(counts.entries()).sort((a, b) => b[1] - a[1])[0]?.[0] ?? '';
}

function combatantMatchesMemberKey(combatant: BattleCombatant | undefined, keys: Set<string>): boolean {
  if (!combatant || keys.size === 0) {
    return false;
  }
  return keys.has(combatantKey(combatant)) || keys.has(combatantName(combatant).toLowerCase());
}

function combatantInAllianceLens(
  combatant: BattleCombatant | undefined,
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): boolean {
  if (!combatant) {
    return false;
  }
  if (combatantMatchesMemberKey(combatant, allianceMemberKeys)) {
    return true;
  }
  return homeAllianceKey !== '' && allianceKey(combatant) === homeAllianceKey;
}

function reportIncludesAllianceLens(
  report: ParsedReport,
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): boolean {
  if (allianceMemberKeys.size === 0 && homeAllianceKey === '') {
    return true;
  }
  return (
    combatantInAllianceLens(report.attacker, allianceMemberKeys, homeAllianceKey) ||
    combatantInAllianceLens(report.defender, allianceMemberKeys, homeAllianceKey)
  );
}

function buildPlayerAggregates(
  list: ParsedReport[],
  selectedPlayerKey: string,
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): PlayerAggregate[] {
  const rows = new Map<string, PlayerAggregate>();

  list.forEach((report) => {
    if (selectedPlayerKey !== allOption) {
      if (combatantMatches(report.attacker, selectedPlayerKey)) {
        addPlayerAggregate(rows, report, 'attacker');
      }
      if (combatantMatches(report.defender, selectedPlayerKey)) {
        addPlayerAggregate(rows, report, 'defender');
      }
      return;
    }

    if (combatantInAllianceLens(report.attacker, allianceMemberKeys, homeAllianceKey)) {
      addPlayerAggregate(rows, report, 'attacker');
    }
    if (combatantInAllianceLens(report.defender, allianceMemberKeys, homeAllianceKey)) {
      addPlayerAggregate(rows, report, 'defender');
    }
  });

  return Array.from(rows.values()).sort((a, b) => b.reports - a.reports || b.unitsKilled - a.unitsKilled);
}

function addPlayerAggregate(rows: Map<string, PlayerAggregate>, report: ParsedReport, side: 'attacker' | 'defender') {
  const combatant = side === 'attacker' ? report.attacker : report.defender;
  const key = combatantKey(combatant);
  if (!key) {
    return;
  }

  const row = rows.get(key) ?? {
    key,
    name: combatantName(combatant),
    alliance: allianceName(combatant),
    reports: 0,
    attacks: 0,
    defenses: 0,
    wins: 0,
    losses: 0,
    unitsLost: 0,
    unitsKilled: 0,
    attackDefendersKilled: 0,
    attackAttackersLost: 0,
    defenseDefendersLost: 0,
    defenseAttackersKilled: 0,
  };
  const attackWon = attackSucceeded(report);
  const attackLost = metricValue(report.metrics, 'attackerLost', 'attackLost');
  const defenseLost = metricValue(report.metrics, 'defenderLost', 'defenseLost');

  row.reports += 1;
  if (side === 'attacker') {
    row.attacks += 1;
    row.unitsLost += attackLost;
    row.unitsKilled += defenseLost;
    row.attackAttackersLost += attackLost;
    row.attackDefendersKilled += defenseLost;
    row.wins += attackWon ? 1 : 0;
    row.losses += attackWon ? 0 : 1;
  } else {
    row.defenses += 1;
    row.unitsLost += defenseLost;
    row.unitsKilled += attackLost;
    row.defenseDefendersLost += defenseLost;
    row.defenseAttackersKilled += attackLost;
    row.wins += attackWon ? 0 : 1;
    row.losses += attackWon ? 1 : 0;
  }

  rows.set(key, row);
}

function buildAllianceAggregates(
  list: ParsedReport[],
  selectedPlayerKey: string,
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): AllianceAggregate[] {
  const rows = new Map<string, AllianceAggregate>();

  list.forEach((report) => {
    opponentSidesForReport(report, selectedPlayerKey, allianceMemberKeys, homeAllianceKey).forEach((side) => {
      addAllianceAggregate(rows, report, side);
    });
  });

  return Array.from(rows.values()).sort((a, b) => b.reports - a.reports || b.unitsKilled - a.unitsKilled);
}

function addAllianceAggregate(rows: Map<string, AllianceAggregate>, report: ParsedReport, side: 'attacker' | 'defender') {
  const combatant = side === 'attacker' ? report.attacker : report.defender;
  const key = allianceKey(combatant);
  if (!key) {
    return;
  }

  const row = rows.get(key) ?? {
    key,
    name: allianceName(combatant),
    reports: 0,
    wins: 0,
    losses: 0,
    unitsLost: 0,
    unitsKilled: 0,
    attackDefendersKilled: 0,
    attackAttackersLost: 0,
    defenseDefendersLost: 0,
    defenseAttackersKilled: 0,
  };
  const attackWon = attackSucceeded(report);
  const attackLost = metricValue(report.metrics, 'attackerLost', 'attackLost');
  const defenseLost = metricValue(report.metrics, 'defenderLost', 'defenseLost');

  row.reports += 1;
  if (side === 'attacker') {
    row.unitsLost += attackLost;
    row.unitsKilled += defenseLost;
    row.defenseDefendersLost += defenseLost;
    row.defenseAttackersKilled += attackLost;
    row.wins += attackWon ? 0 : 1;
    row.losses += attackWon ? 1 : 0;
  } else {
    row.unitsLost += defenseLost;
    row.unitsKilled += attackLost;
    row.attackDefendersKilled += defenseLost;
    row.attackAttackersLost += attackLost;
    row.wins += attackWon ? 1 : 0;
    row.losses += attackWon ? 0 : 1;
  }

  rows.set(key, row);
}

function reportIncludesPlayer(report: ParsedReport, selected: string): boolean {
  return combatantMatches(report.attacker, selected) || combatantMatches(report.defender, selected);
}

function reportMatchesOpponentAlliance(
  report: ParsedReport,
  selectedPlayerKey: string,
  selectedAllianceKey: string,
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): boolean {
  return opponentSidesForReport(report, selectedPlayerKey, allianceMemberKeys, homeAllianceKey).some(
    (side) => allianceKey(combatantForSide(report, side)) === selectedAllianceKey
  );
}

function reportMatchesOpponentPlayer(
  report: ParsedReport,
  selectedPlayerKey: string,
  selectedOpponentPlayerKey: string,
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): boolean {
  return opponentSidesForReport(report, selectedPlayerKey, allianceMemberKeys, homeAllianceKey).some(
    (side) => combatantMatches(combatantForSide(report, side), selectedOpponentPlayerKey)
  );
}

function buildOpponentPlayerOptions(
  reports: ParsedReport[],
  selectedPlayerKey: string,
  selectedAllianceKey: string,
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): FilterOption[] {
  const options = new Map<string, string>();

  reports.forEach((report) => {
    opponentSidesForReport(report, selectedPlayerKey, allianceMemberKeys, homeAllianceKey).forEach((side) => {
      const combatant = combatantForSide(report, side);
      const key = combatantKey(combatant);
      if (!key) {
        return;
      }
      if (selectedAllianceKey !== allOption && allianceKey(combatant) !== selectedAllianceKey) {
        return;
      }
      options.set(key, combatantName(combatant));
    });
  });

  return [
    { value: allOption, label: 'All opponent players' },
    ...Array.from(options.entries())
      .sort((a, b) => a[1].localeCompare(b[1]))
      .map(([value, label]) => ({ value, label })),
  ];
}

function opponentSidesForReport(
  report: ParsedReport,
  selectedPlayerKey: string,
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): CombatantSide[] {
  if (selectedPlayerKey !== allOption) {
    const sides: CombatantSide[] = [];
    if (combatantMatches(report.attacker, selectedPlayerKey)) {
      sides.push('defender');
    }
    if (combatantMatches(report.defender, selectedPlayerKey)) {
      sides.push('attacker');
    }
    return sides;
  }

  const attackerHome = combatantInAllianceLens(report.attacker, allianceMemberKeys, homeAllianceKey);
  const defenderHome = combatantInAllianceLens(report.defender, allianceMemberKeys, homeAllianceKey);

  if (attackerHome && !defenderHome) {
    return ['defender'];
  }
  if (defenderHome && !attackerHome) {
    return ['attacker'];
  }
  return [];
}

function combatantForSide(report: ParsedReport, side: CombatantSide): BattleCombatant | undefined {
  return side === 'attacker' ? report.attacker : report.defender;
}

function reportRole(report: ParsedReport, selectedPlayerKey: string): string {
  if (selectedPlayerKey !== allOption) {
    if (combatantMatches(report.attacker, selectedPlayerKey)) {
      return 'Attacker';
    }
    if (combatantMatches(report.defender, selectedPlayerKey)) {
      return 'Defender';
    }
  }

  const role = stringValue(report.role).toLowerCase();
  if (role.includes('attack')) {
    return 'Attacker';
  }
  if (role.includes('defend')) {
    return 'Defender';
  }
  return 'Unknown';
}

function searchBlob(report: ParsedReport): string {
  return [
    combatantName(report.attacker),
    combatantName(report.defender),
    allianceName(report.attacker),
    allianceName(report.defender),
    battleLocationLabel(report),
    kingdomLabel(report),
    report.battleKey,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
}

function reportID(report?: ParsedReport): string {
  if (!report) {
    return '';
  }

  return [
    report.id,
    report.reportID,
    report.mid,
    report.lid,
    report.battleKey,
    reportTimeMs(report),
    combatantKey(report.attacker),
    combatantKey(report.defender),
  ]
    .filter((part) => part !== undefined && part !== null && String(part) !== '')
    .join(':');
}

function relativeOutcomeLabel(
  report: ParsedReport,
  selectedPlayerKey: string,
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): string {
  const side = friendlySideForReport(report, selectedPlayerKey, allianceMemberKeys, homeAllianceKey);
  const attackWon = attackSucceeded(report);
  if (side === 'attacker') {
    return attackWon ? 'Attack won' : 'Attack lost';
  }
  if (side === 'defender') {
    return attackWon ? 'Defense lost' : 'Defense win';
  }
  return attackWon ? 'Attack won' : 'Attack lost';
}

function splitResultLabel(result: string): { role: 'Attack' | 'Defense'; won: boolean } | null {
  const normalized = result.toLowerCase();
  const role = normalized.startsWith('defense') ? 'Defense' : normalized.startsWith('attack') ? 'Attack' : null;
  if (!role) {
    return null;
  }
  if (normalized.includes('lost')) {
    return { role, won: false };
  }
  if (normalized.includes('won') || normalized.includes('win')) {
    return { role, won: true };
  }
  return null;
}

function friendlySideForReport(
  report: ParsedReport,
  selectedPlayerKey: string,
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): CombatantSide | '' {
  if (selectedPlayerKey !== allOption) {
    if (combatantMatches(report.attacker, selectedPlayerKey)) {
      return 'attacker';
    }
    if (combatantMatches(report.defender, selectedPlayerKey)) {
      return 'defender';
    }
    return '';
  }

  const attackerHome = combatantInAllianceLens(report.attacker, allianceMemberKeys, homeAllianceKey);
  const defenderHome = combatantInAllianceLens(report.defender, allianceMemberKeys, homeAllianceKey);
  if (attackerHome && !defenderHome) {
    return 'attacker';
  }
  if (defenderHome && !attackerHome) {
    return 'defender';
  }
  return '';
}

function attackSucceeded(report: ParsedReport): boolean {
  const attackerSent = metricValue(report.metrics, 'attackerSent', 'attackSent');
  const attackerLost = metricValue(report.metrics, 'attackerLost', 'attackLost');
  return !(attackerSent > 0 && attackerLost >= attackerSent);
}

function kingdomLabel(report: ParsedReport): string {
  const id = numericValue(report.kingdomID ?? report.kingdomId);
  if (id !== null) {
    return kingdomNames[id] ?? `Kingdom ${id}`;
  }
  return 'Kingdom unknown';
}

function battleLocationLabel(report: ParsedReport): string {
  const cleanTargetName = cleanBattleLocation(report.targetName ?? report.castleName ?? '');
  if (cleanTargetName) {
    return cleanTargetName;
  }

  const cleanBattleKey = cleanBattleLocation(report.battleKey ?? '');
  if (cleanBattleKey) {
    return cleanBattleKey;
  }

  return 'Unknown castle';
}

function battleCoordinateLabel(report: ParsedReport): string {
  const coordinates = battleCoordinates(report);
  if (!coordinates) {
    return 'Unknown coordinates';
  }

  return `(${formatNumber(coordinates.x)}, ${formatNumber(coordinates.y)})`;
}

function battleCoordinates(report: ParsedReport): { x: number; y: number } | null {
  const direct = coordinatePair(report.targetX, report.targetY);
  if (direct) {
    return direct;
  }

  const raw = report as unknown as Record<string, unknown>;
  const target = coordinatePair(raw.targetPX, raw.targetPY) ?? coordinatePair(raw.mapX, raw.mapY);
  if (target) {
    return target;
  }

  const bls = isRecord(raw.bls) ? raw.bls : null;
  const ai = bls && isRecord(bls.AI) ? bls.AI : isRecord(raw.AI) ? raw.AI : null;
  return ai ? coordinatePair(ai.X ?? ai.x, ai.Y ?? ai.y) : null;
}

function coordinatePair(rawX: unknown, rawY: unknown): { x: number; y: number } | null {
  const x = numericValue(rawX);
  const y = numericValue(rawY);
  return x !== null && y !== null ? { x, y } : null;
}

function cleanBattleLocation(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    return '';
  }

  const parts = trimmed.split('+');
  const last = parts[parts.length - 1]?.trim();
  return last || trimmed;
}

function reportTimeMs(report: ParsedReport): number | null {
  const dateMs = numericValue(report.dateMs);
  if (dateMs !== null && dateMs > 0) {
    return dateMs;
  }

  const dateUnix = numericValue(report.dateUnix);
  if (dateUnix !== null && dateUnix > 0) {
    return dateUnix > 1_000_000_000_000 ? dateUnix : dateUnix * 1000;
  }

  for (const value of [report.occurredAt, report.timestamp, report.receivedAt]) {
    const parsed = Date.parse(stringValue(value));
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }

  return null;
}

function formatDate(report: ParsedReport): string {
  const time = reportTimeMs(report);
  if (time === null) {
    return 'Unknown';
  }

  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(time));
}

function inputDateFromDaysAgo(days: number): string {
  const date = new Date();
  date.setDate(date.getDate() - days);
  return inputDateFromDate(date);
}

function inputDateFromDate(date: Date): string {
  return date.toISOString().slice(0, 10);
}

function dateStartMs(value: string): number | null {
  if (!value) {
    return null;
  }

  const parsed = new Date(`${value}T00:00:00`).getTime();
  return Number.isFinite(parsed) ? parsed : null;
}

function dateEndMs(value: string): number | null {
  if (!value) {
    return null;
  }

  const parsed = new Date(`${value}T23:59:59.999`).getTime();
  return Number.isFinite(parsed) ? parsed : null;
}

function combatantKey(combatant?: BattleCombatant): string {
  if (!combatant) {
    return '';
  }

  const id = stringValue(combatant.playerID ?? combatant.playerId);
  const name = combatantName(combatant);
  if (id) {
    return id;
  }
  return name === 'Unknown player' ? '' : name.toLowerCase();
}

function combatantMatches(combatant: BattleCombatant | undefined, selected: string): boolean {
  if (!combatant) {
    return false;
  }

  return combatantKey(combatant) === selected || combatantName(combatant).toLowerCase() === selected.toLowerCase();
}

function combatantName(combatant?: BattleCombatant): string {
  const name = stringValue(combatant?.playerName ?? combatant?.name);
  return name || 'Unknown player';
}

function allianceName(combatant?: BattleCombatant): string {
  const name = stringValue(combatant?.allianceName ?? combatant?.alliance);
  const tag = stringValue(combatant?.allianceTag);
  if (tag && name && !name.includes(tag)) {
    return `${name} [${tag}]`;
  }
  return name || tag || 'No alliance';
}

function allianceKey(combatant?: BattleCombatant): string {
  const alliance = allianceName(combatant);
  return alliance === 'No alliance' ? '' : alliance.toLowerCase();
}

function effectsForSide(report: ParsedReport, side: 'commander' | 'castellan'): BattleEffect[] {
  if (side === 'commander' && report.commanderEffects) {
    return report.commanderEffects;
  }
  if (side === 'castellan' && report.castellanEffects) {
    return report.castellanEffects;
  }

  return (report.effects ?? []).filter((effect) => stringValue(effect.side).toLowerCase() === side);
}

function effectLabel(effect: BattleEffect): string {
  return stringValue(effect.label ?? effect.name ?? effect.code) || 'Unknown effect';
}

function effectValue(effect: BattleEffect): string {
  const formatted = stringValue(effect.formattedValue);
  if (formatted) {
    return formatted;
  }

  const value = numericValue(effect.value);
  if (value === null) {
    return '-';
  }

  const prefix = value > 0 ? '+' : '';
  return `${prefix}${value.toFixed(1)}%`;
}

interface EffectComparisonGroup {
  category: string;
  order: number;
  rows: EffectComparisonRow[];
}

interface EffectComparisonRow {
  key: string;
  order: number;
  commander?: BattleEffect;
  castellan?: BattleEffect;
}

interface EffectComparisonBucket {
  key: string;
  category: string;
  order: number;
  commander: BattleEffect[];
  castellan: BattleEffect[];
}

const effectCategoryOrder: Record<string, number> = {
  'Unit effects': 10,
  'Attack effects': 20,
  'Defense unit effects': 30,
  'Defense structure effects': 40,
  'Courtyard effects': 50,
  'Pre-battle effects': 60,
  'Post-battle effects': 70,
  'Other effects': 90,
};

const effectAlignmentOrder: Record<string, number> = {
  meleeStrength: 10,
  rangedStrength: 11,
  courtyardStrength: 12,
  frontStrength: 13,
  flankStrength: 14,
  unitStrength: 15,
  wallProtection: 16,
  gateProtection: 17,
  moatProtection: 18,
  fireDamage: 19,
  frontUnitLimit: 20,
  flankUnitLimit: 21,
  wallUnitLimit: 22,
  finalAssaultCapacity: 23,
  courtyardDefenseCapacity: 24,
  allianceSupportCapacity: 25,
  armyTravelSpeed: 30,
  armyDetection: 31,
  attackWarning: 32,
  sightRadius: 33,
  lootCapacity: 40,
  resources: 41,
  glory: 42,
  honor: 43,
  xp: 44,
  coinLoot: 45,
  equipmentFind: 46,
};

function effectComparisonGroups(commanderEffects: BattleEffect[], castellanEffects: BattleEffect[]): EffectComparisonGroup[] {
  const buckets = new Map<string, EffectComparisonBucket>();
  commanderEffects.forEach((effect) => addEffectComparisonBucket(buckets, effect, 'commander'));
  castellanEffects.forEach((effect) => addEffectComparisonBucket(buckets, effect, 'castellan'));

  const groups = new Map<string, EffectComparisonGroup>();
  Array.from(buckets.values())
    .sort((a, b) => a.order - b.order || a.key.localeCompare(b.key))
    .forEach((bucket) => {
      const commander = sortEffectsForComparison(bucket.commander);
      const castellan = sortEffectsForComparison(bucket.castellan);
      const rowCount = Math.max(commander.length, castellan.length);
      if (rowCount === 0) {
        return;
      }

      const group = groups.get(bucket.category) ?? {
        category: bucket.category,
        order: effectCategoryOrder[bucket.category] ?? 900,
        rows: [],
      };
      for (let index = 0; index < rowCount; index += 1) {
        group.rows.push({
          key: `${bucket.key}-${index}`,
          order: bucket.order + index / 100,
          commander: commander[index],
          castellan: castellan[index],
        });
      }
      groups.set(bucket.category, group);
    });

  return Array.from(groups.values())
    .map((group) => ({
      ...group,
      rows: group.rows.sort((a, b) => a.order - b.order || a.key.localeCompare(b.key)),
    }))
    .sort((a, b) => a.order - b.order || a.category.localeCompare(b.category));
}

function addEffectComparisonBucket(
  buckets: Map<string, EffectComparisonBucket>,
  effect: BattleEffect,
  side: 'commander' | 'castellan'
) {
  const alignmentKey = effectAlignmentKey(effect);
  const valueKind = effectValueKind(effect);
  const category = effectComparisonCategory(effect, alignmentKey);
  const bucketKey = `${category}|${alignmentKey}|${valueKind}`;
  const order = effectComparisonOrder(effect, alignmentKey);
  const bucket = buckets.get(bucketKey) ?? {
    key: bucketKey,
    category,
    order,
    commander: [],
    castellan: [],
  };
  bucket.order = Math.min(bucket.order, order);
  bucket[side].push(effect);
  buckets.set(bucketKey, bucket);
}

function sortEffectsForComparison(effects: BattleEffect[]): BattleEffect[] {
  return effects
    .slice()
    .sort((a, b) => effectSortOrder(a) - effectSortOrder(b) || effectDescription(a).localeCompare(effectDescription(b)));
}

function effectSortOrder(effect: BattleEffect): number {
  return numericValue(effect.sortOrder) ?? 900;
}

function effectComparisonOrder(effect: BattleEffect, alignmentKey: string): number {
  return effectAlignmentOrder[alignmentKey] ?? effectSortOrder(effect);
}

function effectComparisonCategory(effect: BattleEffect, alignmentKey: string): string {
  if (
    [
      'meleeStrength',
      'rangedStrength',
      'courtyardStrength',
      'frontStrength',
      'flankStrength',
      'unitStrength',
    ].includes(alignmentKey)
  ) {
    return 'Unit effects';
  }
  if (['wallProtection', 'gateProtection', 'moatProtection', 'fireDamage'].includes(alignmentKey)) {
    return 'Defense structure effects';
  }
  if (['finalAssaultCapacity', 'courtyardDefenseCapacity', 'allianceSupportCapacity'].includes(alignmentKey)) {
    return 'Courtyard effects';
  }
  if (['armyTravelSpeed', 'armyDetection', 'attackWarning', 'sightRadius'].includes(alignmentKey)) {
    return 'Pre-battle effects';
  }
  if (['lootCapacity', 'resources', 'glory', 'honor', 'xp', 'coinLoot', 'equipmentFind'].includes(alignmentKey)) {
    return 'Post-battle effects';
  }
  return stringValue(effect.category) || 'Other effects';
}

function effectAlignmentKey(effect: BattleEffect): string {
  const text = `${effectLabel(effect)} ${effectDescription(effect)}`.toLowerCase();

  if (text.includes('melee')) {
    return 'meleeStrength';
  }
  if (text.includes('ranged') || text.includes('range ')) {
    return 'rangedStrength';
  }
  if (text.includes('combat strength') && text.includes('courtyard')) {
    return 'courtyardStrength';
  }
  if (text.includes('combat strength') && text.includes('front')) {
    return 'frontStrength';
  }
  if (text.includes('combat strength') && (text.includes('flank') || text.includes('flanks'))) {
    return 'flankStrength';
  }
  if (text.includes('unit combat strength') || text.includes('combat strength for defense units')) {
    return 'unitStrength';
  }
  if (text.includes('wall protection') || text.includes('wall reduction')) {
    return 'wallProtection';
  }
  if (text.includes('gate protection') || text.includes('gate reduction')) {
    return 'gateProtection';
  }
  if (text.includes('moat protection') || text.includes('moat reduction')) {
    return 'moatProtection';
  }
  if (text.includes('fire damage')) {
    return 'fireDamage';
  }
  if (text.includes('unit limit') && text.includes('front')) {
    return 'frontUnitLimit';
  }
  if (text.includes('unit limit') && (text.includes('flank') || text.includes('flanks'))) {
    return 'flankUnitLimit';
  }
  if (text.includes('unit limit') && text.includes('wall')) {
    return 'wallUnitLimit';
  }
  if (text.includes('final assault')) {
    return 'finalAssaultCapacity';
  }
  if (text.includes('courtyard defense capacity') || text.includes('troop capacity in courtyard defense')) {
    return 'courtyardDefenseCapacity';
  }
  if (text.includes('alliance support')) {
    return 'allianceSupportCapacity';
  }
  if (text.includes('later army detection')) {
    return 'armyDetection';
  }
  if (text.includes('attack warning')) {
    return 'attackWarning';
  }
  if (text.includes('sight radius')) {
    return 'sightRadius';
  }
  if (text.includes('travel speed') || text.includes('speed')) {
    return 'armyTravelSpeed';
  }
  if (text.includes('loot capacity')) {
    return 'lootCapacity';
  }
  if (text.includes('resources')) {
    return 'resources';
  }
  if (text.includes('glory')) {
    return 'glory';
  }
  if (text.includes('honor')) {
    return 'honor';
  }
  if (text.includes('xp')) {
    return 'xp';
  }
  if (text.includes('coin')) {
    return 'coinLoot';
  }
  if (text.includes('equipment')) {
    return 'equipmentFind';
  }

  return `effect:${effectLabel(effect).toLowerCase()}:${effectDescription(effect).toLowerCase()}`;
}

function effectValueKind(effect: BattleEffect): string {
  const value = effectValue(effect);
  if (!value || value === '-') {
    return 'none';
  }
  if (value.includes('%')) {
    return 'percent';
  }
  return 'number';
}

function effectDisplayText(effect: BattleEffect): string {
  const displayText = stringValue(effect.displayText);
  if (displayText) {
    return displayText;
  }

  const value = effectValue(effect);
  const label = effectLabel(effect);
  return value && value !== '-' ? `${value} ${label}` : label;
}

function effectDescription(effect: BattleEffect): string {
  const displayText = effectDisplayText(effect);
  const value = effectValue(effect);
  if (value && value !== '-' && displayText.startsWith(value)) {
    return displayText.slice(value.length).trim() || effectLabel(effect);
  }
  return displayText;
}

function metricValue(metrics: BattleMetrics | undefined, ...keys: (keyof BattleMetrics)[]): number {
  if (!metrics) {
    return 0;
  }

  for (const key of keys) {
    const value = numericValue(metrics[key]);
    if (value !== null) {
      return value;
    }
  }

  return 0;
}

function itemsForSide(items: BattleItemDetail[] | undefined, side: 'attacker' | 'defender'): BattleItemDetail[] {
  return (items ?? [])
    .filter((item) => stringValue(item.side).toLowerCase() === side)
    .sort((a, b) => itemSortScore(b) - itemSortScore(a));
}

function itemSortScore(item: BattleItemDetail): number {
  return (numericValue(item.lost) ?? 0) + (numericValue(item.used) ?? 0) + (numericValue(item.amount) ?? 0) / 1000;
}

function itemID(item: BattleItemDetail): number {
  return numericValue(item.wodID ?? item.wodId ?? item.id) ?? 0;
}

function itemKey(item: BattleItemDetail): string {
  return [item.side, item.phase, item.lane, itemID(item), item.amount, item.lost, item.used].filter(Boolean).join(':');
}

function phaseLabel(value: unknown): string {
  const phase = stringValue(value).toLowerCase();
  if (phase === 'wall') {
    return 'Wall';
  }
  if (phase === 'courtyard') {
    return 'Courtyard';
  }
  if (phase === 'support') {
    return 'Support';
  }
  return '';
}

function laneResult(lane: BattleWaveLane): 'HELD' | 'BREACHED' {
  const parsed = stringValue(lane.result).toUpperCase();
  if (parsed === 'HELD' || parsed === 'BREACHED') {
    return parsed;
  }

  const defenderStart = lane.defenderStart ?? 0;
  const defenderLost = lane.defenderLost ?? 0;
  const attackerStart = lane.attackerStart ?? 0;
  const attackerLost = lane.attackerLost ?? 0;

  if (defenderStart > 0 && defenderLost < defenderStart) {
    return 'HELD';
  }
  if (defenderStart > 0 && defenderLost >= defenderStart) {
    return 'BREACHED';
  }
  if (attackerStart > 0 && attackerLost < attackerStart) {
    return 'BREACHED';
  }

  return 'HELD';
}

function laneLabel(lane: BattleWaveLane, index: number): string {
  const label = stringValue(lane.lane);
  if (label) {
    return label;
  }

  return ['Left flank', 'Middle front', 'Right flank'][index] ?? `Lane ${index + 1}`;
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value);
}

function formatRatio(numerator: number, denominator: number): string {
  if (denominator <= 0) {
    return '--';
  }
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 2, minimumFractionDigits: 2 }).format(numerator / denominator);
}

function formatTradeRatio(opponentLost: number, ourLost: number): string {
  if (ourLost <= 0) {
    return opponentLost > 0 ? '∞ : 1' : '--';
  }

  const value = opponentLost / ourLost;
  return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2, minimumFractionDigits: 2 }).format(value)} : 1`;
}

function numericValue(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value.replace(/,/g, ''));
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function stringValue(value: unknown): string {
  if (typeof value === 'string') {
    return value.trim();
  }
  if (typeof value === 'number' && Number.isFinite(value)) {
    return String(value);
  }
  return '';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export default BattleStatsView;
