import React, { useEffect, useMemo, useState } from 'react';
import {
  ArrowRight,
  BarChart3,
  CalendarDays,
  Castle,
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
  '/Data/BattleReports.jsonl',
  '/BattleReports.jsonl',
  '/api/battle-reports',
  '/api/battleReports',
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
                        <ResultBadge result={relativeOutcomeLabel(report, selectedPlayer, allianceMemberKeys, homeAllianceKey)} />
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
}

interface AllianceAggregate {
  key: string;
  name: string;
  reports: number;
  wins: number;
  losses: number;
  unitsLost: number;
  unitsKilled: number;
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
            <th className="px-4 py-3 font-semibold text-right">Lost</th>
            <th className="px-4 py-3 font-semibold text-right">Killed</th>
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
              <td className="px-4 py-3 text-right text-error font-semibold">{formatNumber(row.unitsLost)}</td>
              <td className="px-4 py-3 text-right text-info font-semibold">{formatNumber(row.unitsKilled)}</td>
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
            <th className="px-4 py-3 font-semibold text-right">Lost</th>
            <th className="px-4 py-3 font-semibold text-right">Killed</th>
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
              <td className="px-4 py-3 text-right text-error font-semibold">{formatNumber(row.unitsLost)}</td>
              <td className="px-4 py-3 text-right text-info font-semibold">{formatNumber(row.unitsKilled)}</td>
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

const ReportDetailPage: React.FC<{ report: ParsedReport; outcome: string; onBack: () => void }> = ({
  report,
  outcome,
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
    <ReportDetails report={report} outcome={outcome} />
  </div>
);

const ReportDetails: React.FC<{ report: ParsedReport; outcome: string }> = ({ report, outcome }) => {
  const commanderEffects = effectsForSide(report, 'commander');
  const castellanEffects = effectsForSide(report, 'castellan');
  const attackerUnits = itemsForSide(report.topUnits, 'attacker').slice(0, 12);
  const defenderUnits = itemsForSide(report.topUnits, 'defender').slice(0, 12);
  const attackerTools = itemsForSide(report.supportTools, 'attacker').slice(0, 12);
  const defenderTools = itemsForSide(report.supportTools, 'defender').slice(0, 12);

  return (
    <div className="space-y-4">
      <Card variant="solid">
        <CardContent className="grid gap-4 xl:grid-cols-[1fr_auto_1fr_auto] xl:items-center">
          <CombatantHero label="Attacker" combatant={report.attacker} accent="danger" />
          <div className="text-center px-4">
            <ResultBadge result={outcome} />
            <div className="text-2xl font-bold text-text-main mt-2">{battleLocationLabel(report)}</div>
            <div className="text-sm text-text-muted">{kingdomLabel(report)}</div>
          </div>
          <CombatantHero label="Defender" combatant={report.defender} accent="info" />
          <div className="grid grid-cols-2 gap-3 text-sm">
            <MetaItem label="Date" value={formatDate(report)} />
            <MetaItem label="Battle type" value={report.battleType ?? 'Castle battle'} />
          </div>
        </CardContent>
      </Card>

      <Card variant="solid">
        <CardHeader>
          <div>
            <CardTitle>All Unit Stats</CardTitle>
            <p className="text-xs text-text-muted mt-1">Report totals inferred from parsed battle data</p>
          </div>
          <BarChart3 className="w-5 h-5 text-primary" />
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-6 gap-3">
            <MetricTile label="Attacker sent" value={metricValue(report.metrics, 'attackerSent', 'attackSent')} />
            <MetricTile label="Attacker lost" value={metricValue(report.metrics, 'attackerLost', 'attackLost')} tone="danger" />
            <MetricTile label="Attackers killed" value={metricValue(report.metrics, 'attackersKilled')} tone="success" />
            <MetricTile label="Defender stationed" value={metricValue(report.metrics, 'defenderStationed')} tone="info" />
            <MetricTile label="Defender lost" value={metricValue(report.metrics, 'defenderLost', 'defenseLost')} tone="danger" />
            <MetricTile label="Defenders killed" value={metricValue(report.metrics, 'defendersKilled')} tone="success" />
          </div>
        </CardContent>
      </Card>

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

      <div className="grid xl:grid-cols-2 gap-4">
        <EffectList title="Commander" subtitle={combatantName(report.attacker)} effects={commanderEffects} tone="danger" />
        <EffectList title="Castellan" subtitle={combatantName(report.defender)} effects={castellanEffects} tone="info" />
      </div>

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

const CombatantHero: React.FC<{ label: string; combatant?: BattleCombatant; accent: 'danger' | 'info' }> = ({
  label,
  combatant,
  accent,
}) => {
  const accentClass = accent === 'danger' ? 'text-error border-error/30' : 'text-info border-info/30';

  return (
    <div className={`border-l-2 pl-4 ${accentClass}`}>
      <div className="text-xs uppercase tracking-wider text-text-muted font-semibold">{label}</div>
      <div className="text-xl font-bold text-text-main mt-1">{combatantName(combatant)}</div>
      <div className="text-sm text-text-muted">{allianceName(combatant)}</div>
      {combatant?.castleName && (
        <div className="text-xs text-text-muted mt-1">{combatant.castleName}</div>
      )}
    </div>
  );
};

const MetaItem: React.FC<{ label: string; value: string }> = ({ label, value }) => (
  <div className="border border-border-base rounded-global px-3 py-2 bg-bg-app">
    <div className="text-[10px] uppercase tracking-wider text-text-muted font-semibold">{label}</div>
    <div className="text-sm text-text-main mt-1">{value}</div>
  </div>
);

const MetricTile: React.FC<{ label: string; value: number; tone?: 'neutral' | 'success' | 'danger' | 'info' }> = ({
  label,
  value,
  tone = 'neutral',
}) => {
  const toneClass = {
    neutral: 'text-text-main',
    success: 'text-success',
    danger: 'text-error',
    info: 'text-info',
  }[tone];

  return (
    <div className="border border-border-base rounded-global px-3 py-3 bg-bg-app">
      <div className="text-xs text-text-muted">{label}</div>
      <div className={`text-lg font-bold mt-1 ${toneClass}`}>{formatNumber(value)}</div>
    </div>
  );
};

const EffectList: React.FC<{
  title: string;
  subtitle: string;
  effects: BattleEffect[];
  tone: 'danger' | 'info';
}> = ({ title, subtitle, effects, tone }) => {
  const valueClass = tone === 'danger' ? 'text-error' : 'text-info';
  const groups = effectGroups(effects);

  return (
    <Card variant="solid">
      <CardHeader>
        <div>
          <CardTitle>{title}</CardTitle>
          <p className="text-xs text-text-muted mt-1">{subtitle}</p>
        </div>
        <Badge variant="secondary">{effects.length} effects</Badge>
      </CardHeader>
      <CardContent>
        {effects.length > 0 ? (
          <div className="space-y-4">
            {groups.map((group) => (
              <div key={group.category} className="space-y-2">
                <div className="text-[11px] uppercase tracking-wider font-bold text-text-muted">
                  {group.category}
                </div>
                <div className="divide-y divide-border-base rounded-global border border-border-base bg-bg-app">
                  {group.effects.map((effect, index) => (
                    <div key={`${effectLabel(effect)}-${index}`} className="px-3 py-2.5">
                      <span className={`text-sm font-medium ${valueClass}`}>{effectDisplayText(effect)}</span>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="text-sm text-text-muted py-8 text-center">No parsed effects for this side.</div>
        )}
      </CardContent>
    </Card>
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
      <div className="grid sm:grid-cols-2 gap-2">
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

  return (
    <div className={`border border-border-base rounded-global bg-bg-app flex items-center gap-2 ${compact ? 'p-2' : 'p-2.5'}`}>
      {kind === 'unit' ? (
        <UnitImage unitId={id} size={compact ? 30 : 40} showLevel className="rounded-md" />
      ) : (
        <ToolImage toolId={id} size={compact ? 30 : 40} className="rounded-md" />
      )}
      <div className="min-w-0 flex-1">
        <div className="text-xs font-semibold text-text-main truncate" title={name}>{name}</div>
        <div className="flex items-center gap-2 text-[11px] text-text-muted mt-0.5">
          <span>x{formatNumber(amount)}</span>
          {phaseLabel(item.phase) && <span>{phaseLabel(item.phase)}</span>}
        </div>
      </div>
      <div className={`text-xs font-bold whitespace-nowrap ${valueClass}`}>
        {kind === 'unit' ? `-${formatNumber(delta)}` : `used ${formatNumber(delta || amount)}`}
      </div>
    </div>
  );
};

const WaveRow: React.FC<{ wave: BattleWave; index: number }> = ({ wave, index }) => (
  <div className="border border-border-base rounded-global bg-bg-app p-3">
    <div className="flex items-center justify-between gap-3 mb-3">
      <div className="font-semibold text-text-main">Wave {wave.wave ?? wave.index ?? index + 1}</div>
      <Badge variant={waveResult(wave) === 'HELD' ? 'success' : 'warning'}>{waveResult(wave)}</Badge>
    </div>
    <div className="grid xl:grid-cols-3 gap-3">
      {(wave.lanes ?? []).map((lane, laneIndex) => (
        <LaneDetailCard key={`${lane.lane ?? laneIndex}-${laneIndex}`} lane={lane} laneIndex={laneIndex} />
      ))}
    </div>
    {(!wave.lanes || wave.lanes.length === 0) && (
      <div className="text-sm text-text-muted">No lane details parsed for this wave.</div>
    )}
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
      <div className="grid grid-cols-2 gap-2 mt-3 text-sm">
        <div className="rounded-global border border-border-base bg-bg-app px-2 py-2">
          <div className="text-[11px] text-text-muted">Attack lost</div>
          <div className="font-semibold text-error">{formatNumber(lane.attackerLost ?? 0)}</div>
        </div>
        <div className="rounded-global border border-border-base bg-bg-app px-2 py-2">
          <div className="text-[11px] text-text-muted">Def lost</div>
          <div className="font-semibold text-info">{formatNumber(lane.defenderLost ?? 0)}</div>
        </div>
      </div>
      <div className="mt-3 space-y-3">
        <LaneItemStrip title="Attackers lost" items={attackerUnits} kind="unit" valueClass="text-error" />
        <LaneItemStrip title="Attack tools used" items={attackerTools} kind="tool" valueClass="text-error" />
        <LaneItemStrip title="Defenders lost" items={defenderUnits} kind="unit" valueClass="text-info" />
        <LaneItemStrip title="Defense tools used" items={defenderTools} kind="tool" valueClass="text-info" />
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
      <div className="grid gap-1.5">
        {items.slice(0, 6).map((item, index) => (
          <BattleItemChip key={`${title}-${itemKey(item)}-${index}`} item={item} kind={kind} valueClass={valueClass} compact />
        ))}
      </div>
      {items.length > 6 && (
        <div className="text-[11px] text-text-muted mt-1">+{items.length - 6} more</div>
      )}
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
  };
  const attackWon = attackSucceeded(report);
  const attackLost = metricValue(report.metrics, 'attackerLost', 'attackLost');
  const defenseLost = metricValue(report.metrics, 'defenderLost', 'defenseLost');

  row.reports += 1;
  if (side === 'attacker') {
    row.attacks += 1;
    row.unitsLost += attackLost;
    row.unitsKilled += defenseLost;
    row.wins += attackWon ? 1 : 0;
    row.losses += attackWon ? 0 : 1;
  } else {
    row.defenses += 1;
    row.unitsLost += defenseLost;
    row.unitsKilled += attackLost;
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
  };
  const attackWon = attackSucceeded(report);
  const attackLost = metricValue(report.metrics, 'attackerLost', 'attackLost');
  const defenseLost = metricValue(report.metrics, 'defenderLost', 'defenseLost');

  row.reports += 1;
  if (side === 'attacker') {
    row.unitsLost += attackLost;
    row.unitsKilled += defenseLost;
    row.wins += attackWon ? 1 : 0;
    row.losses += attackWon ? 0 : 1;
  } else {
    row.unitsLost += defenseLost;
    row.unitsKilled += attackLost;
    row.wins += attackWon ? 0 : 1;
    row.losses += attackWon ? 1 : 0;
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

interface EffectGroup {
  category: string;
  effects: BattleEffect[];
}

function effectGroups(effects: BattleEffect[]): EffectGroup[] {
  const byCategory = new Map<string, BattleEffect[]>();

  effects
    .slice()
    .sort((a, b) => effectSortOrder(a) - effectSortOrder(b) || effectLabel(a).localeCompare(effectLabel(b)))
    .forEach((effect) => {
      const category = stringValue(effect.category) || 'Other effects';
      const rows = byCategory.get(category) ?? [];
      rows.push(effect);
      byCategory.set(category, rows);
    });

  return Array.from(byCategory.entries()).map(([category, rows]) => ({ category, effects: rows }));
}

function effectSortOrder(effect: BattleEffect): number {
  return numericValue(effect.sortOrder) ?? 900;
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

function waveResult(wave: BattleWave): 'HELD' | 'BREACHED' {
  const lanes = wave.lanes ?? [];
  return lanes.some((lane) => laneResult(lane) === 'BREACHED') ? 'BREACHED' : 'HELD';
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
