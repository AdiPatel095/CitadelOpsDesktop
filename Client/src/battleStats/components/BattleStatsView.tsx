import React, { useEffect, useId, useMemo, useState } from 'react';
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
import DetailBackButton from '../../components/DetailBackButton';
import { useMetadata } from '../../context/MetadataContext';
import { useLastKnownSnapshot } from '../../context/LastKnownSnapshotContext';
import { FrontendWebsocket } from '../../Websocket';
import { TROOP_METADATA } from '../../config/Constants';
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

const REPORT_ROWS_PAGE_SIZE = 250;

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
  const [visibleReportLimit, setVisibleReportLimit] = useState(REPORT_ROWS_PAGE_SIZE);

  const loadReports = async () => {
    setIsLoading(true);
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
      FrontendWebsocket.showAlert(
        'red',
        error instanceof Error ? error.message : 'Could not load battle reports'
      );
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
  const ownPlayerKeys = useMemo(() => ownPlayerKeysFromSnapshot(snapshot), [snapshot]);
  const playerHasAllianceEntry = useMemo(
    () => allianceMemberKeys.size > 0 && (ownPlayerKeys.size === 0 || setsOverlap(allianceMemberKeys, ownPlayerKeys)),
    [allianceMemberKeys, ownPlayerKeys]
  );
  const visibilityPlayerKeys = useMemo(
    () => (playerHasAllianceEntry ? allianceMemberKeys : ownPlayerKeys),
    [playerHasAllianceEntry, allianceMemberKeys, ownPlayerKeys]
  );
  const rosterHomeAllianceKey = useMemo(
    () => resolveHomeAllianceKey(reports, allianceMemberKeys),
    [reports, allianceMemberKeys]
  );
  const homeAllianceKey = playerHasAllianceEntry ? rosterHomeAllianceKey : '';
  const reportsWithBothPlayers = useMemo(
    () => reports.filter(hasBothPlayers),
    [reports]
  );
  const scopedReports = useMemo(
    () => reportsWithBothPlayers.filter((report) => reportIncludesAllianceLens(report, visibilityPlayerKeys, homeAllianceKey)),
    [reportsWithBothPlayers, visibilityPlayerKeys, homeAllianceKey]
  );

  const playerOptions = useMemo(
    () => buildAlliancePlayerOptions(scopedReports, allianceMembers, visibilityPlayerKeys, homeAllianceKey),
    [scopedReports, allianceMembers, visibilityPlayerKeys, homeAllianceKey]
  );

  const allianceOptions = useMemo(() => {
    const allianceOptionReports = scopedReports
      .filter((report) =>
        selectedOpponentPlayer === allOption ||
        reportMatchesOpponentPlayer(report, selectedPlayer, selectedOpponentPlayer, visibilityPlayerKeys, homeAllianceKey)
      );
    const rows = buildAllianceAggregates(
      allianceOptionReports,
      selectedPlayer,
      visibilityPlayerKeys,
      homeAllianceKey
    );

    return [
      { value: allOption, label: 'All opponent alliances' },
      ...rows.map((row) => ({ value: row.key, label: row.name })),
    ];
  }, [scopedReports, selectedPlayer, selectedOpponentPlayer, visibilityPlayerKeys, homeAllianceKey]);

  const opponentPlayerOptions = useMemo(
    () => buildOpponentPlayerOptions(scopedReports, selectedPlayer, selectedAlliance, visibilityPlayerKeys, homeAllianceKey),
    [scopedReports, selectedPlayer, selectedAlliance, visibilityPlayerKeys, homeAllianceKey]
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

  useEffect(() => {
    setVisibleReportLimit(REPORT_ROWS_PAGE_SIZE);
  }, [searchTerm, selectedAlliance, selectedOpponentPlayer, selectedPlayer, selectedResult, selectedRole, startDate, endDate]);

  const filteredReports = useMemo(() => {
    const startMs = dateStartMs(startDate);
    const endMs = dateEndMs(endDate);
    const query = searchTerm.trim().toLowerCase();

    return scopedReports
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
          relativeOutcomeLabel(report, selectedPlayer, visibilityPlayerKeys, homeAllianceKey) !== selectedResult
        ) {
          return false;
        }
        if (selectedPlayer !== allOption && !reportIncludesPlayer(report, selectedPlayer)) {
          return false;
        }
        if (selectedPlayer === allOption && !reportIncludesAllianceLens(report, visibilityPlayerKeys, homeAllianceKey)) {
          return false;
        }
        if (
          selectedAlliance !== allOption &&
          !reportMatchesOpponentAlliance(report, selectedPlayer, selectedAlliance, visibilityPlayerKeys, homeAllianceKey)
        ) {
          return false;
        }
        if (
          selectedOpponentPlayer !== allOption &&
          !reportMatchesOpponentPlayer(report, selectedPlayer, selectedOpponentPlayer, visibilityPlayerKeys, homeAllianceKey)
        ) {
          return false;
        }
        if (selectedRole !== allOption && reportRole(report, selectedPlayer, visibilityPlayerKeys, homeAllianceKey) !== selectedRole) {
          return false;
        }
        if (query && !searchBlob(report).includes(query)) {
          return false;
        }
        return true;
      })
      .sort((a, b) => (reportTimeMs(b) ?? 0) - (reportTimeMs(a) ?? 0));
  }, [scopedReports, searchTerm, selectedAlliance, selectedOpponentPlayer, selectedPlayer, selectedResult, selectedRole, startDate, endDate, visibilityPlayerKeys, homeAllianceKey]);

  const summary = useMemo(
    () => summarizeReports(filteredReports, selectedPlayer, visibilityPlayerKeys, homeAllianceKey),
    [filteredReports, selectedPlayer, visibilityPlayerKeys, homeAllianceKey]
  );
  const detailReport = useMemo(() => {
    if (!selectedReportID) {
      return null;
    }

    return filteredReports.find((report) => reportID(report) === selectedReportID) ?? null;
  }, [filteredReports, selectedReportID]);
  const playerAggregates = useMemo(
    () => buildPlayerAggregates(filteredReports, selectedPlayer, visibilityPlayerKeys, homeAllianceKey),
    [filteredReports, selectedPlayer, visibilityPlayerKeys, homeAllianceKey]
  );
  const allianceAggregates = useMemo(
    () => buildAllianceAggregates(filteredReports, selectedPlayer, visibilityPlayerKeys, homeAllianceKey),
    [filteredReports, selectedPlayer, visibilityPlayerKeys, homeAllianceKey]
  );
  const visibleReports = useMemo(
    () => filteredReports.slice(0, visibleReportLimit),
    [filteredReports, visibleReportLimit]
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
        outcome={relativeOutcomeLabel(detailReport, selectedPlayer, visibilityPlayerKeys, homeAllianceKey)}
        perspectiveSide={friendlySideForReport(detailReport, selectedPlayer, visibilityPlayerKeys, homeAllianceKey)}
        onBack={() => setSelectedReportID(null)}
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-4 xl:flex-row xl:items-start">
        <aside className="xl:w-[21.5rem] shrink-0">
          <Card variant="solid" className="liquid-prominent-header-card">
            <CardHeader className="liquid-card-header-prominent">
              <div>
                <CardTitle>Battle Stats</CardTitle>
                <p className="mt-1.5 text-xs font-semibold text-text-muted">{sourceLabel}</p>
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
            <CardContent className="liquid-prominent-header-content battle-filters-grid">
              <Input
                value={searchTerm}
                onChange={(event) => setSearchTerm(event.target.value)}
                placeholder="Find player, alliance, castle"
                leftIcon={<Search className="w-4 h-4" />}
              />

              <FilterField label="Date range" icon={<CalendarDays className="w-4 h-4" />}>
                <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
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

            </CardContent>
          </Card>
        </aside>

        <section className="flex-1 min-w-0 space-y-4">
          <div className="battle-summary-grid">
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

          <Card variant="solid" className="liquid-prominent-header-card">
            <CardHeader className="liquid-card-header-prominent">
              <div>
                <CardTitle>Reports</CardTitle>
                <p className="text-xs text-text-muted mt-1">
                  Showing {formatNumber(visibleReports.length)} of {formatNumber(filteredReports.length)} filtered reports
                  <span className="text-text-muted/70"> · {formatNumber(scopedReports.length)} parsed</span>
                </p>
              </div>
              <Badge variant="secondary">{isLoading ? 'Loading' : 'Ready'}</Badge>
            </CardHeader>
            <div className="liquid-prominent-header-content liquid-prominent-header-content-flush overflow-x-auto">
              <table className="battle-table w-full text-sm">
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
                  {visibleReports.map((report) => (
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
                        <ReportResultBadges result={relativeOutcomeLabel(report, selectedPlayer, visibilityPlayerKeys, homeAllianceKey)} />
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
                          className="battle-stats-flat-control h-9 w-9 border-primary/40 text-primary hover:border-primary hover:bg-primary/10"
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
              {visibleReports.length < filteredReports.length && (
                <div className="flex justify-center border-t border-border-base/70 px-4 py-4">
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => setVisibleReportLimit((current) => current + REPORT_ROWS_PAGE_SIZE)}
                  >
                    Show {formatNumber(Math.min(REPORT_ROWS_PAGE_SIZE, filteredReports.length - visibleReports.length))} more
                  </Button>
                </div>
              )}
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
  const badgeClass = size === 'lg' ? 'px-4 py-1.5 text-sm md:text-base' : 'battle-stats-flat-control';

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
  <Card variant="solid" className="liquid-prominent-header-card">
    <CardHeader className="liquid-card-header-prominent">
      <div>
        <CardTitle>Player Aggregate</CardTitle>
        <p className="text-xs text-text-muted mt-1">Filtered battle totals by player</p>
      </div>
      <Badge variant="secondary">{rows.length} players</Badge>
    </CardHeader>
    <div className="liquid-prominent-header-content liquid-prominent-header-content-flush">
      <div className="overflow-x-auto">
        <table className="battle-aggregate-table w-full text-sm">
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
    </div>
  </Card>
);

const AllianceAggregateTable: React.FC<{ rows: AllianceAggregate[] }> = ({ rows }) => (
  <Card variant="solid" className="liquid-prominent-header-card">
    <CardHeader className="liquid-card-header-prominent">
      <div>
        <CardTitle>Alliance Aggregate</CardTitle>
        <p className="text-xs text-text-muted mt-1">Filtered battle totals by alliance</p>
      </div>
      <Badge variant="secondary">{rows.length} alliances</Badge>
    </CardHeader>
    <div className="liquid-prominent-header-content liquid-prominent-header-content-flush">
      <div className="overflow-x-auto">
        <table className="battle-aggregate-table w-full text-sm">
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
    </div>
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
    <BattleDetailsHeader report={report} outcome={outcome} onBack={onBack} />
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
      <UnitStatsPanel report={report} perspectiveSide={perspectiveSide} />

      <ForcesAccordion
        attacker={report.attacker}
        attackerUnits={attackerUnits}
        attackerTools={attackerTools}
        defender={report.defender}
        defenderUnits={defenderUnits}
        defenderTools={defenderTools}
      />

      <EffectComparison
        commanderName={combatantName(report.attacker)}
        commanderEffects={commanderEffects}
        castellanName={combatantName(report.defender)}
        castellanEffects={castellanEffects}
      />

      {report.waves && report.waves.length > 0 && (
        <Card variant="solid" className="liquid-prominent-header-card">
          <CardHeader className="liquid-card-header-prominent">
            <CardTitle>Wall Waves</CardTitle>
            <Badge variant="secondary">{report.waves.length} waves</Badge>
          </CardHeader>
          <CardContent className="liquid-prominent-header-content space-y-3">
            {report.waves.map((wave, index) => (
              <WaveRow key={`${wave.wave ?? wave.index ?? index}-${index}`} wave={wave} index={index} />
            ))}
          </CardContent>
        </Card>
      )}
    </div>
  );
};

const BattleDetailsHeader: React.FC<{ report: ParsedReport; outcome: string; onBack: () => void }> = ({
  report,
  outcome,
  onBack,
}) => {
  return (
    <Card variant="solid" className="battle-report-dossier">
      <div className="battle-report-dossier-top">
        <div className="battle-report-title-block">
          <div className="battle-report-mark">
            <Swords className="h-6 w-6" />
          </div>
          <div className="min-w-0">
            <div className="battle-report-eyebrow">Battle report dossier</div>
            <CardTitle className="battle-report-dossier-title">{battleLocationLabel(report)}</CardTitle>
          </div>
        </div>
        <DetailBackButton label="Back to battle stats" onClick={onBack} />
      </div>

      <div className="battle-report-matchup">
        <BattleBannerSide label="Attacker" combatant={report.attacker} tone="danger" />

        <div className="battle-report-outcome-seal">
          <div className="battle-report-seal-ring">
            <Swords className="h-7 w-7" />
          </div>
          <div className="battle-report-seal-label">Final result</div>
          <ReportResultBadges result={outcome} size="lg" className="battle-report-result-badges" />
          <div className="battle-report-seal-location">{battleLocationLabel(report)}</div>
        </div>

        <BattleBannerSide label="Defender" combatant={report.defender} tone="info" align="right" />
      </div>

      <div className="battle-report-intel-strip">
        <BannerFact icon={<CalendarDays className="h-4 w-4" />} label="Date" value={formatDate(report)} />
        <BannerFact icon={<MapPin className="h-4 w-4" />} label="Coordinates" value={battleCoordinateLabel(report)} />
        <BannerFact icon={<Shield className="h-4 w-4" />} label="Kingdom" value={kingdomLabel(report)} />
      </div>
    </Card>
  );
};

const BattleBannerSide: React.FC<{
  label: string;
  combatant?: BattleCombatant;
  tone: 'danger' | 'info';
  align?: 'left' | 'right';
}> = ({ label, combatant, tone, align = 'left' }) => {
  const toneClass = tone === 'danger' ? 'battle-report-side-danger' : 'battle-report-side-info';
  const alignClass = align === 'right' ? 'battle-report-side-defender' : 'battle-report-side-attacker';

  return (
    <div className={`battle-report-side ${toneClass} ${alignClass}`}>
      <div className="battle-report-side-label">{label}</div>
      <div className="battle-report-side-name">{combatantName(combatant)}</div>
      <div className="battle-report-side-alliance">{allianceName(combatant)}</div>
      {combatant?.castleName && (
        <div className="battle-report-side-castle">
          {combatant.castleName}
        </div>
      )}
    </div>
  );
};

const BannerFact: React.FC<{ icon: React.ReactNode; label: string; value: string }> = ({ icon, label, value }) => (
  <div className="battle-report-intel-item">
    <span className="battle-report-intel-icon">{icon}</span>
    <div className="min-w-0">
      <div className="battle-report-intel-label">{label}</div>
      <div className="battle-report-intel-value">{value}</div>
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
  const defenderLossesByRole = defenderLossRoleTotals(report);
  const isDefenseView = side === 'defender';
  const ourForce = isDefenseView ? defenderStationed : attackerSent;
  const ourLost = isDefenseView ? defenderLost : attackerLost;
  const opponentForce = isDefenseView ? attackerSent : defenderStationed;
  const opponentLost = isDefenseView ? attackerLost : defenderLost;
  const totalLosses = attackerLost + defenderLost;
  const tradeRatio = formatTradeRatio(opponentLost, ourLost);
  const tradeTone = opponentLost > ourLost ? 'success' : opponentLost < ourLost ? 'danger' : 'neutral';

  return (
    <Card variant="solid" className="liquid-prominent-header-card">
      <CardHeader className="liquid-card-header-prominent">
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
      <CardContent className="liquid-prominent-header-content">
        <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
          <MetricTile
            label={isDefenseView ? 'Our stationed' : 'Our sent'}
            value={ourForce}
            tone={isDefenseView ? 'info' : 'neutral'}
          />
          {isDefenseView ? (
            <SplitMetricTile
              label="Our losses"
              leftLabel="Attack units"
              leftValue={defenderLossesByRole.attack}
              rightLabel="Defense units"
              rightValue={defenderLossesByRole.defense}
              tone="danger"
              caption={defenderLossesByRole.unknown > 0 ? `Unclassified ${formatNumber(defenderLossesByRole.unknown)}` : undefined}
            />
          ) : (
            <MetricTile label="Our losses" value={ourLost} tone="danger" />
          )}
          <MetricTile
            label={isDefenseView ? 'Opponent sent' : 'Opponent stationed'}
            value={opponentForce}
            tone={isDefenseView ? 'neutral' : 'info'}
          />
          {isDefenseView ? (
            <MetricTile label="Opponent losses" value={opponentLost} tone="success" />
          ) : (
            <SplitMetricTile
              label="Opponent losses"
              leftLabel="Attack units"
              leftValue={defenderLossesByRole.attack}
              rightLabel="Defense units"
              rightValue={defenderLossesByRole.defense}
              tone="success"
              caption={defenderLossesByRole.unknown > 0 ? `Unclassified ${formatNumber(defenderLossesByRole.unknown)}` : undefined}
            />
          )}
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

const SplitMetricTile: React.FC<{
  label: string;
  leftLabel: string;
  leftValue: number;
  rightLabel: string;
  rightValue: number;
  tone?: 'neutral' | 'success' | 'danger' | 'info';
  caption?: string;
}> = ({
  label,
  leftLabel,
  leftValue,
  rightLabel,
  rightValue,
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

  return (
    <div className={`border rounded-global px-3 py-3 bg-bg-app ${borderClass}`}>
      <div className="text-xs text-text-muted">{label}</div>
      <div className="mt-2 grid grid-cols-2 gap-2">
        <div className="rounded-global border border-border-base/70 bg-bg-card px-2 py-2">
          <div className={`text-lg font-bold leading-none ${toneClass}`}>{formatNumber(leftValue)}</div>
          <div className="mt-1 text-[10px] font-semibold uppercase tracking-wider text-text-muted">{leftLabel}</div>
        </div>
        <div className="rounded-global border border-border-base/70 bg-bg-card px-2 py-2">
          <div className={`text-lg font-bold leading-none ${toneClass}`}>{formatNumber(rightValue)}</div>
          <div className="mt-1 text-[10px] font-semibold uppercase tracking-wider text-text-muted">{rightLabel}</div>
        </div>
      </div>
      {caption && <div className="mt-1 text-[11px] text-text-muted">{caption}</div>}
    </div>
  );
};

const CollapsibleDetailCard: React.FC<{
  title: string;
  subtitle?: string;
  defaultOpen?: boolean;
  children: React.ReactNode;
}> = ({ title, subtitle, defaultOpen = false, children }) => {
  const [isOpen, setIsOpen] = useState(defaultOpen);
  const contentId = useId();

  return (
    <Card variant="solid" className="liquid-prominent-header-card">
      <CardHeader className="liquid-card-header-prominent !p-0">
        <button
          type="button"
          className="flex min-h-[4.75rem] w-full items-center justify-between gap-3 rounded-global px-6 py-5 text-left transition-colors hover:text-primary"
          aria-expanded={isOpen}
          aria-controls={contentId}
          onClick={() => setIsOpen((current) => !current)}
        >
          <div className="min-w-0">
            <CardTitle>{title}</CardTitle>
            {subtitle && <p className="mt-1 truncate text-xs text-text-muted">{subtitle}</p>}
          </div>
          {isOpen ? (
            <ChevronDown className="h-5 w-5 shrink-0 text-text-muted" aria-hidden="true" />
          ) : (
            <ChevronRight className="h-5 w-5 shrink-0 text-text-muted" aria-hidden="true" />
          )}
        </button>
      </CardHeader>
      {isOpen && (
        <CardContent id={contentId} className="liquid-prominent-header-content">
          {children}
        </CardContent>
      )}
    </Card>
  );
};

const EffectComparison: React.FC<{
  commanderName: string;
  commanderEffects: BattleEffect[];
  castellanName: string;
  castellanEffects: BattleEffect[];
}> = ({ commanderName, commanderEffects, castellanName, castellanEffects }) => {
  const visibleCommanderEffects = commanderEffects.filter(isVisibleEffectComparisonEffect);
  const visibleCastellanEffects = castellanEffects.filter(isVisibleEffectComparisonEffect);
  const groups = effectComparisonGroups(visibleCommanderEffects, visibleCastellanEffects);
  const totalEffects = visibleCommanderEffects.length + visibleCastellanEffects.length;

  return (
    <CollapsibleDetailCard title="Commander / Castellan" subtitle={`${commanderName} vs ${castellanName}`}>
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
                      <EffectComparisonCell effects={row.commander} side="commander" />
                      <EffectComparisonCell effects={row.castellan} side="castellan" />
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
    </CollapsibleDetailCard>
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

function effectCellKey(effect: BattleEffect, index: number): string {
  return `${effectLabel(effect)}|${effectDescription(effect)}|${effectValue(effect)}|${index}`;
}

const EffectComparisonCell: React.FC<{ effects?: BattleEffect[]; side: 'commander' | 'castellan' }> = ({ effects, side }) => {
  const visibleEffects = effects ?? [];
  if (visibleEffects.length === 0) {
    return <div className="min-h-12 px-3 py-2.5 text-xs font-semibold text-text-muted/50">-</div>;
  }

  return (
    <div className="flex min-h-12 flex-col gap-2 px-3 py-2.5">
      {visibleEffects.map((effect, index) => {
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
          <div key={effectCellKey(effect, index)} className="flex items-start justify-between gap-3">
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
      })}
    </div>
  );
};

const ForcesAccordion: React.FC<{
  attacker?: BattleCombatant;
  attackerUnits: BattleItemDetail[];
  attackerTools: BattleItemDetail[];
  defender?: BattleCombatant;
  defenderUnits: BattleItemDetail[];
  defenderTools: BattleItemDetail[];
}> = ({
  attacker,
  attackerUnits,
  attackerTools,
  defender,
  defenderUnits,
  defenderTools,
}) => (
  <CollapsibleDetailCard title="Forces" subtitle={`${combatantName(attacker)} vs ${combatantName(defender)}`}>
    <div className="grid gap-4 xl:grid-cols-2">
      <ForceSidePanel
        title="Attacker"
        combatant={attacker}
        units={attackerUnits}
        tools={attackerTools}
        tone="danger"
      />
      <ForceSidePanel
        title="Defender"
        combatant={defender}
        units={defenderUnits}
        tools={defenderTools}
        tone="info"
      />
    </div>
  </CollapsibleDetailCard>
);

const ForceSidePanel: React.FC<{
  title: string;
  combatant?: BattleCombatant;
  units: BattleItemDetail[];
  tools: BattleItemDetail[];
  tone: 'danger' | 'info';
}> = ({ title, combatant, units, tools, tone }) => {
  const accentClass = tone === 'danger' ? 'text-error' : 'text-info';

  return (
    <section className="min-w-0 rounded-global border border-border-base bg-bg-app p-4">
      <div className="mb-4 min-w-0">
        <div className={`text-xs font-bold uppercase tracking-wider ${accentClass}`}>{title}</div>
        <div className="mt-1 truncate text-sm font-semibold text-text-main">{combatantName(combatant)}</div>
      </div>
      <div className="space-y-4">
        <RosterSection title="Units fought" items={units} kind="unit" valueClass={accentClass} />
        <RosterSection title="Tools used" items={tools} kind="tool" valueClass={accentClass} />
      </div>
    </section>
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

function setsOverlap(left: Set<string>, right: Set<string>): boolean {
  for (const value of left) {
    if (right.has(value)) {
      return true;
    }
  }
  return false;
}

function ownPlayerKeysFromSnapshot(snapshot: Record<string, unknown> | null): Set<string> {
  const keys = new Set<string>();
  const gameState = isRecord(snapshot?.gameState) ? snapshot.gameState : null;
  const player = isRecord(gameState?.player) ? gameState.player : null;
  const id = stringValue(
    gameState?.playerId ??
    gameState?.playerID ??
    gameState?.PlayerID ??
    player?.id ??
    player?.ID ??
    player?.playerID ??
    player?.playerId
  );

  if (id) {
    keys.add(id);
  }

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
    const explicitSide = allianceMemberKeys.size === 0 && homeAllianceKey === ''
      ? explicitOwnSideForReport(report)
      : '';
    if (explicitSide) {
      const combatant = combatantForSide(report, explicitSide);
      const key = combatantKey(combatant);
      if (key) {
        options.set(key, combatantName(combatant));
      }
      return;
    }

    [report.attacker, report.defender].forEach((combatant) => {
      const key = combatantKey(combatant);
      if (key && combatantInAllianceLens(combatant, allianceMemberKeys, homeAllianceKey)) {
        options.set(key, combatantName(combatant));
      }
    });
  });

  return [
    { value: allOption, label: 'Own reports' },
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
    return explicitOwnSideForReport(report) !== '';
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

    const explicitSide = allianceMemberKeys.size === 0 && homeAllianceKey === ''
      ? explicitOwnSideForReport(report)
      : '';
    if (explicitSide) {
      addPlayerAggregate(rows, report, explicitSide);
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

  if (allianceMemberKeys.size === 0 && homeAllianceKey === '') {
    const ownSide = explicitOwnSideForReport(report);
    if (ownSide === 'attacker') {
      return ['defender'];
    }
    if (ownSide === 'defender') {
      return ['attacker'];
    }
    return [];
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

function reportRole(
  report: ParsedReport,
  selectedPlayerKey: string,
  allianceMemberKeys: Set<string>,
  homeAllianceKey: string
): string {
  const side = friendlySideForReport(report, selectedPlayerKey, allianceMemberKeys, homeAllianceKey);
  if (side === 'attacker') {
    return 'Attacker';
  }
  if (side === 'defender') {
    return 'Defender';
  }

  const ownSide = explicitOwnSideForReport(report);
  if (ownSide === 'attacker') {
    return 'Attacker';
  }
  if (ownSide === 'defender') {
    return 'Defender';
  }
  return 'Unknown';
}

function explicitOwnSideForReport(report: ParsedReport): CombatantSide | '' {
  const record = report as unknown as Record<string, unknown>;
  const role = [
    report.role,
    record.ownRole,
    record.playerRole,
    record.viewerRole,
    record.perspective,
    record.perspectiveRole,
    record.perspectiveSide,
    record.friendlySide,
    record.ownSide,
  ]
    .map(stringValue)
    .find(Boolean)
    ?.toLowerCase() ?? '';

  if (role.includes('attack')) {
    return 'attacker';
  }
  if (role.includes('defend')) {
    return 'defender';
  }
  return '';
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

  if (allianceMemberKeys.size === 0 && homeAllianceKey === '') {
    return explicitOwnSideForReport(report);
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
  const phaseResult = attackSucceededFromBattlePhases(report);
  if (phaseResult !== null) {
    return phaseResult;
  }
  const result = stringValue(report.result ?? report.outcome).toLowerCase();
  if (result.includes('defeat') || result.includes('attack lost') || result.includes('defense win')) {
    return false;
  }
  if (result.includes('victory') || result.includes('attack won') || result.includes('defense lost')) {
    return true;
  }
  const attackerSent = metricValue(report.metrics, 'attackerSent', 'attackSent');
  const attackerLost = metricValue(report.metrics, 'attackerLost', 'attackLost');
  return !(attackerSent > 0 && attackerLost >= attackerSent);
}

function attackSucceededFromBattlePhases(report: ParsedReport): boolean | null {
  const wall = wallPhaseResult(report);
  if (!wall.hasAttackLane) {
    return null;
  }
  if (!wall.breached) {
    return false;
  }
  return attackSucceededFromCourtyard(report);
}

function wallPhaseResult(report: ParsedReport): { hasAttackLane: boolean; breached: boolean } {
  let hasAttackLane = false;
  for (const wave of report.waves ?? []) {
    for (const lane of wave.lanes ?? []) {
      if ((lane.attackerStart ?? 0) <= 0 && (lane.attackerLost ?? 0) <= 0) {
        continue;
      }
      hasAttackLane = true;
      if (laneResult(lane) === 'BREACHED') {
        return { hasAttackLane, breached: true };
      }
    }
  }
  return { hasAttackLane, breached: false };
}

function attackSucceededFromCourtyard(report: ParsedReport): boolean | null {
  const attacker = battlePhaseTotals(report.topUnits ?? [], 'attacker', 'courtyard');
  const defender = battlePhaseTotals(report.topUnits ?? [], 'defender', 'courtyard');
  if (attacker.started <= 0 && defender.started <= 0) {
    return null;
  }
  if (attacker.started > 0 && attacker.lost >= attacker.started) {
    return false;
  }
  if (defender.started > 0 && defender.lost < defender.started) {
    return false;
  }
  if (attacker.started > 0 && attacker.lost < attacker.started) {
    return true;
  }
  if (defender.started > 0 && defender.lost >= defender.started) {
    return true;
  }
  return null;
}

function battlePhaseTotals(
  items: BattleItemDetail[],
  side: 'attacker' | 'defender',
  phase: string
): { started: number; lost: number } {
  return items.reduce(
    (totals, item) => {
      if (stringValue(item.side).toLowerCase() !== side || stringValue(item.phase).toLowerCase() !== phase) {
        return totals;
      }
      totals.started += numericValue(item.amount) ?? 0;
      totals.lost += numericValue(item.lost) ?? 0;
      return totals;
    },
    { started: 0, lost: 0 }
  );
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
  commander: BattleEffect[];
  castellan: BattleEffect[];
}

interface EffectComparisonBucket {
  key: string;
  category: string;
  order: number;
  commander: BattleEffect[];
  castellan: BattleEffect[];
}

const attackDefenseUnitEffectCategory = 'Bonus Effects';
const attackDefenseUnitUnkeyedBucketKey = `${attackDefenseUnitEffectCategory}|unkeyed`;

const effectCategoryOrder: Record<string, number> = {
  'Unit effects': 10,
  [attackDefenseUnitEffectCategory]: 20,
  'Structure Effects': 40,
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
  courtyardCapacity: 23,
  allianceSupportCapacity: 25,
  armyTravelSpeed: 30,
  attackDetectionWindow: 31,
  sightRadius: 33,
  lootCapacity: 40,
  resources: 41,
  glory: 42,
  honor: 43,
  xp: 44,
  coinLoot: 45,
  equipmentFind: 46,
};

const hiddenEffectComparisonKeys = new Set([
  'lootCapacity',
  'resources',
  'glory',
  'honor',
  'xp',
  'coinLoot',
  'equipmentFind',
]);

function isVisibleEffectComparisonEffect(effect: BattleEffect): boolean {
  return !hiddenEffectComparisonKeys.has(effectAlignmentKey(effect));
}

function effectComparisonGroups(commanderEffects: BattleEffect[], castellanEffects: BattleEffect[]): EffectComparisonGroup[] {
  const buckets = new Map<string, EffectComparisonBucket>();
  const pairedAttackDefenseKeys = pairedAttackDefenseUnitKeys(commanderEffects, castellanEffects);
  commanderEffects.forEach((effect) => addEffectComparisonBucket(buckets, effect, 'commander', pairedAttackDefenseKeys));
  castellanEffects.forEach((effect) => addEffectComparisonBucket(buckets, effect, 'castellan', pairedAttackDefenseKeys));

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
      const commanderRows = singleEffectRows(commander, rowCount);
      const castellanRows = singleEffectRows(castellan, rowCount);

      const group = groups.get(bucket.category) ?? {
        category: bucket.category,
        order: effectCategoryOrder[bucket.category] ?? 900,
        rows: [],
      };
      for (let index = 0; index < rowCount; index += 1) {
        group.rows.push({
          key: `${bucket.key}-${index}`,
          order: bucket.order + index / 100,
          commander: commanderRows[index] ?? [],
          castellan: castellanRows[index] ?? [],
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

function singleEffectRows(effects: BattleEffect[], rowCount: number): BattleEffect[][] {
  return Array.from({ length: rowCount }, (_, index) => {
    const effect = effects[index];
    return effect ? [effect] : [];
  });
}

function addEffectComparisonBucket(
  buckets: Map<string, EffectComparisonBucket>,
  effect: BattleEffect,
  side: 'commander' | 'castellan',
  pairedAttackDefenseKeys: Set<string>
) {
  const alignmentKey = effectAlignmentKey(effect);
  const valueKind = effectValueKind(effect);
  const category = effectComparisonCategory(effect, alignmentKey);
  const comparisonKey = `${alignmentKey}|${valueKind}`;
  const hasPairedAttackDefenseKey = pairedAttackDefenseKeys.has(comparisonKey);
  const bucketKey = category === attackDefenseUnitEffectCategory && !hasPairedAttackDefenseKey
    ? attackDefenseUnitUnkeyedBucketKey
    : `${category}|${comparisonKey}`;
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

function pairedAttackDefenseUnitKeys(commanderEffects: BattleEffect[], castellanEffects: BattleEffect[]): Set<string> {
  const commanderKeys = attackDefenseUnitKeys(commanderEffects);
  const castellanKeys = attackDefenseUnitKeys(castellanEffects);
  return new Set(Array.from(commanderKeys).filter((key) => castellanKeys.has(key)));
}

function attackDefenseUnitKeys(effects: BattleEffect[]): Set<string> {
  const keys = new Set<string>();
  effects.forEach((effect) => {
    const alignmentKey = effectAlignmentKey(effect);
    if (alignmentKey.startsWith('effect:')) {
      return;
    }
    if (effectComparisonCategory(effect, alignmentKey) === attackDefenseUnitEffectCategory) {
      keys.add(`${alignmentKey}|${effectValueKind(effect)}`);
    }
  });
  return keys;
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
    return 'Structure Effects';
  }
  if (['courtyardCapacity', 'allianceSupportCapacity'].includes(alignmentKey)) {
    return 'Courtyard effects';
  }
  if (['armyTravelSpeed', 'attackDetectionWindow', 'sightRadius'].includes(alignmentKey)) {
    return 'Pre-battle effects';
  }
  if (['lootCapacity', 'resources', 'glory', 'honor', 'xp', 'coinLoot', 'equipmentFind'].includes(alignmentKey)) {
    return 'Post-battle effects';
  }
  const category = stringValue(effect.category) || 'Other effects';
  if (category === 'Attack effects' || category === 'Defense unit effects') {
    return attackDefenseUnitEffectCategory;
  }
  return category;
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
  if (
    text.includes('final assault') ||
    text.includes('courtyard defense capacity') ||
    text.includes('troop capacity in courtyard defense')
  ) {
    return 'courtyardCapacity';
  }
  if (text.includes('alliance support')) {
    return 'allianceSupportCapacity';
  }
  if (text.includes('later army detection')) {
    return 'attackDetectionWindow';
  }
  if (text.includes('attack warning')) {
    return 'attackDetectionWindow';
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

function defenderLossRoleTotals(report: ParsedReport): { attack: number; defense: number; unknown: number } {
  const totals = { attack: 0, defense: 0, unknown: 0 };
  const defenderLost = metricValue(report.metrics, 'defenderLost', 'defenseLost');
  const defenderUnitLosses = itemsForSide(report.topUnits, 'defender');

  defenderUnitLosses.forEach((item) => {
    const lost = numericValue(item.lost) ?? 0;
    if (lost <= 0) {
      return;
    }

    const role = TROOP_METADATA[itemID(item)]?.role;
    if (role === 'attack') {
      totals.attack += lost;
    } else if (role === 'defense') {
      totals.defense += lost;
    } else {
      totals.unknown += lost;
    }
  });

  const categorized = totals.attack + totals.defense + totals.unknown;
  if (defenderLost > categorized) {
    totals.unknown += defenderLost - categorized;
  }

  return totals;
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
  const defenderStart = lane.defenderStart ?? 0;
  const defenderLost = lane.defenderLost ?? 0;
  const attackerStart = lane.attackerStart ?? 0;
  const attackerLost = lane.attackerLost ?? 0;

  if (attackerStart > 0 && attackerLost >= attackerStart) {
    return 'HELD';
  }
  if (defenderStart > 0 && defenderLost < defenderStart) {
    return 'HELD';
  }
  if (defenderStart > 0 && defenderLost >= defenderStart) {
    return 'BREACHED';
  }
  if (attackerStart > 0 && attackerLost < attackerStart) {
    return 'BREACHED';
  }

  const parsed = stringValue(lane.result).toUpperCase();
  if (parsed === 'HELD' || parsed === 'BREACHED') {
    return parsed;
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
