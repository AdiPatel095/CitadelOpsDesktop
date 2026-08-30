import React, { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Bot, Castle, Clock3, Coins, RefreshCw, ShieldCheck, Swords } from 'lucide-react';
import { useCitadelAPI } from '../../api/ApiContext';
import type { GameStateV2, ScalableEventScoreV2 } from '../../api/Contracts';
import { castleOptionsFromState } from '../../api/Selectors';
import {
  ATTACK_PRESETS_SECTION,
  parseAttackPresetDocument,
  summarizeAttackPreset,
} from '../../attackPresets/AttackPresetTypes';
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Card, Input, Modal, ModalTitle, Select, SettingsModal, Switch } from '../../components/ui';
import {
  AUTO_ADVISOR_MAX_ATTACKS,
  AUTO_ADVISOR_SECTION,
  clampAutoAdvisorInteger,
  defaultAutoAdvisorClientState,
  parseAutoAdvisorClientState,
  type AutoAdvisorClientStateV1,
} from '../AutoAdvisorClientState';
import { eventDifficultyName, useEventDifficultyOptions } from '../EventDifficultyOptions';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';

interface AutoAdvisorSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const AutoAdvisorSettingsModal: React.FC<AutoAdvisorSettingsModalProps> = ({ isOpen, onClose }) => {
  const { state, configuration, submitIntent, updateConfiguration } = useCitadelAPI();
  const [draft, setDraft] = useState<AutoAdvisorClientStateV1>(defaultAutoAdvisorClientState);
  const [saving, setSaving] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [activating, setActivating] = useState(false);
  const [activationOpen, setActivationOpen] = useState(false);
  const [activationAcknowledged, setActivationAcknowledged] = useState(false);

  const castles = useMemo(() => castleOptionsFromState(state).filter((castle) => castle.kingdomId === 0), [state]);
  const presetDocument = useMemo(
    () => parseAttackPresetDocument(configuration?.sections[ATTACK_PRESETS_SECTION]),
    [configuration?.sections],
  );
  const selectedPreset = presetDocument.presets.find((preset) => preset.id === draft.presetId);
  const presetSummary = selectedPreset ? summarizeAttackPreset(selectedPreset) : null;
  const completedAchievements = state?.player.achievements?.completed ?? {};
  const achievementsObserved = Boolean(state?.player.achievements?.observedAt);
  const difficultyCatalog = useEventDifficultyOptions(isOpen, [72, 80], completedAchievements);
  const nomadDifficulties = difficultyCatalog.optionsByEvent['72'] ?? [];
  const samuraiDifficulties = difficultyCatalog.optionsByEvent['80'] ?? [];
  const nomadSelectionAvailable = nomadDifficulties.some((option) => option.value === String(draft.nomadDifficultyId));
  const samuraiSelectionAvailable = samuraiDifficulties.some((option) => option.value === String(draft.samuraiDifficultyId));
  const activeEvent = useMemo(() => activeAdvisorEvent(state), [state]);
  const eventLabel = activeEvent?.eventId === 72 ? 'Nomad' : activeEvent?.eventId === 80 ? 'Samurai' : 'Event';
  const eventTokenID = activeEvent
    ? activeEvent.advisorCurrencyId || (activeEvent.eventId === 72 ? 77 : 78)
    : 0;
  const eventTokens = currencyAmount(state, eventTokenID);
  const universalTokens = currencyAmount(state, 76);
  const advisorActive = activeEvent?.advisorActive === true;
  const activationHasToken = activeEvent?.advisorFree === true || eventTokens > 0 || universalTokens > 0;
  const canActivate = Boolean(
    activeEvent
    && !advisorActive
    && activeEvent.difficultyId
    && activationHasToken
    && state?.session.socketReady
    && !activating,
  );
  const run = state?.advisor?.run;
  const summary = state?.advisor?.summary;
  const summaryObserved = Boolean(summary?.observedAt && Date.parse(summary.observedAt) > 0);

  useEffect(() => {
    if (!isOpen) return;
    setDraft(parseAutoAdvisorClientState(configuration?.sections[AUTO_ADVISOR_SECTION]));
  }, [configuration?.sections, isOpen]);

  const canSave = draft.sourceCastleId > 0
    && Boolean(draft.presetId)
    && nomadSelectionAvailable
    && samuraiSelectionAvailable
    && draft.maxAttackCount >= 1
    && (!(draft.horseTravelBoostId === 1008 || draft.horseTravelBoostId === 1009) || draft.rubyCostPerAttack > 0);

  const save = async () => {
    if (saving || !canSave) return;
    setSaving(true);
    try {
      await updateConfiguration(AUTO_ADVISOR_SECTION, draft);
      Notifications.success('Auto Advisor settings saved. No advisor token was consumed.');
      onClose();
    } catch (error) {
      Notifications.error(error instanceof Error ? error.message : 'Could not save Auto Advisor settings.');
    } finally {
      setSaving(false);
    }
  };

  const refreshOverview = async () => {
    if (refreshing || !activeEvent) return;
    setRefreshing(true);
    try {
      await submitIntent('advisor.overview.refresh', {}, { actor: 'ui:auto-advisor' });
      Notifications.success('Advisor overview refresh requested.');
    } catch {
      // The API context already presents the server error.
    } finally {
      setRefreshing(false);
    }
  };

  const activateAdvisor = async () => {
    if (!canActivate || !activeEvent || !activationAcknowledged) return;
    setActivating(true);
    try {
      await submitIntent('advisor.activate', {
        eventId: activeEvent.eventId,
        confirmedTokenSpend: true,
      }, { actor: 'ui:auto-advisor' });
      setActivationOpen(false);
      setActivationAcknowledged(false);
      Notifications.success(`${eventLabel} advisor activation submitted.`);
    } catch {
      // The API context already presents the server error.
    } finally {
      setActivating(false);
    }
  };

  const setInteger = <K extends keyof AutoAdvisorClientStateV1>(
    key: K,
    value: unknown,
    minimum: number,
    maximum: number,
    fallback: number,
  ) => {
    setDraft((current) => ({
      ...current,
      [key]: clampAutoAdvisorInteger(value, minimum, maximum, fallback),
    }));
  };

  const openActivation = () => {
    if (!canActivate) return;
    setActivationAcknowledged(false);
    setActivationOpen(true);
  };

  return (
    <>
      <SettingsModal
        isOpen={isOpen}
        onClose={() => { if (!saving && !activating) onClose(); }}
        maxWidth="3xl"
        title="Auto Advisor"
        icon={<Bot className="h-5 w-5" />}
        description="One guarded Nomad or Samurai advisor run per event"
        onSave={() => void save()}
        isSaving={saving}
        saveDisabled={!canSave}
      >
        <div className="space-y-3">
          <Card variant="solid" className="p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <div className="flex flex-wrap items-center gap-2 text-sm font-black text-text-main">
                  <ShieldCheck className="h-4 w-4 text-primary" /> Advisor access
                  <Badge variant={advisorActive ? 'success' : activeEvent ? 'warning' : 'secondary'}>
                    {advisorActive ? `${eventLabel} unlocked` : activeEvent ? `${eventLabel} locked` : 'No supported event'}
                  </Badge>
                </div>
                <p className="mt-1 text-xs text-text-muted">
                  Saving or enabling automation never consumes a token. Activation is a separate confirmed command.
                </p>
              </div>
              <div className="flex flex-wrap gap-2">
                {advisorActive ? (
                  <Button
                    variant="outline"
                    size="sm"
                    isLoading={refreshing}
                    onClick={() => void refreshOverview()}
                    leftIcon={<RefreshCw className="h-3.5 w-3.5" />}
                  >
                    Refresh overview
                  </Button>
                ) : (
                  <Button variant="danger" size="sm" disabled={!canActivate} onClick={openActivation}>
                    Activate advisor
                  </Button>
                )}
              </div>
            </div>
            {activeEvent ? (
              <div className="mt-3 grid gap-2 border-t border-border-base pt-3 sm:grid-cols-3">
                <LiveValue label="Event difficulty" value={activeEvent.difficultyId ? String(activeEvent.difficultyId) : 'Not selected'} />
                <LiveValue label={`${eventLabel} tokens`} value={eventTokens.toLocaleString()} />
                <LiveValue label="Universal tokens" value={universalTokens.toLocaleString()} />
              </div>
            ) : (
              <p className="mt-3 border-t border-border-base pt-3 text-xs text-warning">A running Nomad or Samurai event is required.</p>
            )}
          </Card>

          {run || summaryObserved ? (
            <Card variant="solid" className="p-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="text-sm font-black text-text-main">Live advisor run</div>
                <Badge variant={run?.status === 'running' ? 'primary' : run?.status === 'completed' ? 'success' : run?.status === 'cancelled' ? 'warning' : 'secondary'}>
                  {run?.status ?? 'Overview only'}
                </Badge>
              </div>
              <div className="mt-3 grid gap-2 sm:grid-cols-4">
                <LiveValue label="Current attack" value={run ? `${run.currentAttack.toLocaleString()} / ${run.requestedAttacks.toLocaleString()}` : '—'} />
                <LiveValue label="Wins / defeats" value={`${(summary?.wins ?? 0).toLocaleString()} / ${(summary?.defeats ?? 0).toLocaleString()}`} />
                <LiveValue label="Units lost" value={(summary?.unitsLost ?? 0).toLocaleString()} />
                <LiveValue label="Tools lost" value={(summary?.toolsLost ?? 0).toLocaleString()} />
              </div>
              {run?.status === 'cancelled' ? (
                <p className="mt-3 text-xs text-warning">The game accepted MCM for this chain. Its remaining advisor attacks are cancelled, and CitadelOps will not restart it automatically.</p>
              ) : null}
            </Card>
          ) : null}

          <Card variant="solid" className="p-4">
            <div className="grid gap-4 md:grid-cols-2">
              <label className="block">
                <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Castle className="h-3.5 w-3.5" /> Source castle</span>
                <Select
                  value={draft.sourceCastleId > 0 ? String(draft.sourceCastleId) : ''}
                  onChange={(value) => setDraft((current) => ({ ...current, sourceCastleId: Number(value) || 0 }))}
                  options={castles.map((castle) => ({ value: String(castle.id), label: `${castle.name} · ${castle.x}:${castle.y}` }))}
                  placeholder="Choose a Great Empire castle"
                  menuGrowToViewport
                />
              </label>
              <label className="block">
                <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Swords className="h-3.5 w-3.5" /> Advisor attack preset</span>
                <Select
                  value={draft.presetId}
                  onChange={(presetId) => setDraft((current) => ({ ...current, presetId }))}
                  options={presetDocument.presets.map((preset) => ({ value: preset.id, label: preset.name }))}
                  placeholder={presetDocument.presets.length ? 'Choose a CitadelOps preset' : 'Create an Attack Preset first'}
                  disabled={presetDocument.presets.length === 0}
                  menuGrowToViewport
                />
              </label>
              <HorseTravelBoostSelect
                className="block md:col-span-2"
                value={draft.horseTravelBoostId}
                onChange={(horseTravelBoostId) => setDraft((current) => ({ ...current, horseTravelBoostId }))}
              />
            </div>
            {presetSummary ? (
              <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-border-base pt-3">
                <span className="mr-1 text-xs text-text-muted">Reserved for every requested attack</span>
                <Badge variant="outline">{presetSummary.waves} waves</Badge>
                <Badge variant="outline">{presetSummary.troops.toLocaleString()} troops</Badge>
                <Badge variant="outline">{presetSummary.tools.toLocaleString()} tools</Badge>
              </div>
            ) : null}
          </Card>

          <Card variant="solid" className="p-4">
            <div className="mb-3 flex items-start justify-between gap-3">
              <div>
                <div className="text-sm font-black text-text-main">Automated event difficulty</div>
                <p className="mt-1 text-xs text-text-muted">If the event has not started, Auto Advisor selects the configured unlocked difficulty first.</p>
              </div>
              <Badge variant="outline">{achievementsObserved ? 'Achievements synced' : 'Syncing achievements'}</Badge>
            </div>
            <div className="grid gap-4 md:grid-cols-2">
              <DifficultySelect
                label="Nomad"
                value={nomadSelectionAvailable ? draft.nomadDifficultyId : 0}
                options={nomadDifficulties}
                through={eventDifficultyName(nomadDifficulties, Number(nomadDifficulties.at(-1)?.value))}
                onChange={(nomadDifficultyId) => setDraft((current) => ({ ...current, nomadDifficultyId }))}
              />
              <DifficultySelect
                label="Samurai"
                value={samuraiSelectionAvailable ? draft.samuraiDifficultyId : 0}
                options={samuraiDifficulties}
                through={eventDifficultyName(samuraiDifficulties, Number(samuraiDifficulties.at(-1)?.value))}
                onChange={(samuraiDifficultyId) => setDraft((current) => ({ ...current, samuraiDifficultyId }))}
              />
            </div>
            {difficultyCatalog.loading ? <p className="mt-3 text-xs text-text-muted">Loading official event difficulties…</p> : null}
            {difficultyCatalog.error ? <p className="mt-3 text-xs text-danger">{difficultyCatalog.error}</p> : null}
          </Card>

          <Card variant="solid" className="p-4">
            <div className="mb-3 flex items-center gap-2 text-sm font-black text-text-main"><Clock3 className="h-4 w-4 text-primary" /> Run sizing</div>
            <div className="grid gap-4 sm:grid-cols-2">
              <NumberField
                label="Maximum attacks"
                value={draft.maxAttackCount}
                min={1}
                max={AUTO_ADVISOR_MAX_ATTACKS}
                suffix="AAC"
                onChange={(value) => setInteger('maxAttackCount', value, 1, AUTO_ADVISOR_MAX_ATTACKS, AUTO_ADVISOR_MAX_ATTACKS)}
              />
              <NumberField
                label="Stop before event end"
                value={Math.round(draft.minimumRemainingSec / 60)}
                min={0}
                max={1440}
                suffix="min"
                onChange={(value) => setInteger('minimumRemainingSec', Number(value) * 60, 0, 86400, 1800)}
              />
            </div>
            <p className="mt-3 text-[11px] text-text-muted">The emitted AAC is the smallest safe count allowed by this limit, event time, complete preset copies, coins, rubies, feathers, and one-command cooldown skips. It never exceeds 9,999.</p>
          </Card>

          <Card variant="solid" className="p-4">
            <div className="mb-3 flex items-center gap-2 text-sm font-black text-text-main"><Coins className="h-4 w-4 text-primary" /> Resource gates</div>
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
              <NumberField label="All-in coins / attack" value={draft.coinCostPerAttack} min={1} suffix="coins" onChange={(value) => setInteger('coinCostPerAttack', value, 1, Number.MAX_SAFE_INTEGER, 500)} />
              <NumberField label="Keep coins" value={draft.minimumCoinReserve} min={0} suffix="reserve" onChange={(value) => setInteger('minimumCoinReserve', value, 0, Number.MAX_SAFE_INTEGER, 0)} />
              <NumberField label="Rubies / attack" value={draft.rubyCostPerAttack} min={0} suffix="rubies" onChange={(value) => setInteger('rubyCostPerAttack', value, 0, Number.MAX_SAFE_INTEGER, 0)} />
              <NumberField label="Keep rubies" value={draft.minimumRubyReserve} min={0} suffix="reserve" onChange={(value) => setInteger('minimumRubyReserve', value, 0, Number.MAX_SAFE_INTEGER, 0)} />
              <NumberField label="Keep feathers" value={draft.minimumFeatherReserve} min={0} suffix="PTT" onChange={(value) => setInteger('minimumFeatherReserve', value, 0, Number.MAX_SAFE_INTEGER, 0)} />
            </div>
            <div className="mt-4 grid grid-cols-3 gap-2 border-t border-border-base pt-3">
              {([['MS5', '60m'], ['MS6', '5h'], ['MS7', '24h']] as const).map(([key, label]) => (
                <NumberField
                  key={key}
                  label={`Keep ${label}`}
                  value={draft.timeSkipReserve[key] ?? 0}
                  min={0}
                  suffix={key}
                  onChange={(value) => setDraft((current) => ({
                    ...current,
                    timeSkipReserve: {
                      ...current.timeSkipReserve,
                      [key]: clampAutoAdvisorInteger(value, 0, Number.MAX_SAFE_INTEGER, 0),
                    },
                  }))}
                />
              ))}
            </div>
            <p className="mt-3 text-[11px] text-text-muted">The coin value is the conservative total per attack, including a coin horse when selected. Ruby horses require a positive observed ruby cost per attack; travel-feather runs use one PTT per attack instead.</p>
          </Card>

          <p className="rounded-global border border-border-base bg-bg-app/40 px-4 py-3 text-xs text-text-muted">
            Auto Advisor launches only after the game reports the advisor unlocked. It never buys or activates a token on its own. A failed, completed, or MCM-cancelled run is terminal for that event and is not silently replaced.
          </p>
        </div>
      </SettingsModal>

      <Modal
        isOpen={activationOpen}
        onClose={() => { if (!activating) setActivationOpen(false); }}
        maxWidth="md"
        title={<ModalTitle icon={<AlertTriangle className="h-5 w-5" />}>Activate {eventLabel} advisor</ModalTitle>}
        footer={(
          <>
            <Button variant="ghost" disabled={activating} onClick={() => setActivationOpen(false)}>Cancel</Button>
            <Button
              variant="danger"
              isLoading={activating}
              disabled={!canActivate || !activationAcknowledged}
              onClick={() => void activateAdvisor()}
            >
              {activeEvent?.advisorFree ? 'Activate advisor' : 'Activate and spend token'}
            </Button>
          </>
        )}
      >
        <div className="space-y-4">
          <p className="text-sm leading-relaxed text-text-main">
            {activeEvent?.advisorFree
              ? 'The game reports this activation as free for the current event.'
              : `This command can consume one paid ${eventLabel} advisor token or one universal advisor token. The game chooses the eligible token.`}
          </p>
          <div className="rounded-global border border-warning/30 bg-warning/10 p-4 text-xs leading-relaxed text-warning">
            Activation unlocks advisor attacks for the rest of this event. If Auto Advisor is enabled and its settings are valid, it may launch the single resource-sized run immediately afterward.
          </div>
          <div className="flex items-start justify-between gap-4 rounded-global border border-border-base p-4">
            <div>
              <div className="text-sm font-bold text-text-main">Confirm paid-feature activation</div>
              <p className="mt-1 text-xs text-text-muted">
                {activeEvent?.advisorFree
                  ? 'I understand this unlock may allow enabled automation to launch immediately.'
                  : 'I understand this action may consume an advisor token acquired through a real-money purchase.'}
              </p>
            </div>
            <Switch checked={activationAcknowledged} onChange={setActivationAcknowledged} ariaLabel="Confirm advisor token spend" />
          </div>
        </div>
      </Modal>
    </>
  );
};

const LiveValue: React.FC<{ label: string; value: string }> = ({ label, value }) => (
  <div className="rounded-xl border border-border-base bg-bg-app/45 p-3">
    <div className="text-[10px] font-black uppercase tracking-wider text-text-muted">{label}</div>
    <div className="mt-1 font-mono text-sm font-bold text-text-main">{value}</div>
  </div>
);

interface DifficultySelectProps {
  label: string;
  value: number;
  options: Array<{ value: string; label: string }>;
  through: string;
  onChange: (value: number) => void;
}

const DifficultySelect: React.FC<DifficultySelectProps> = ({ label, value, options, through, onChange }) => (
  <label className="block">
    <span className="mb-1.5 flex items-center justify-between gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
      {label}
      <span className="normal-case tracking-normal text-primary">Through {through}</span>
    </span>
    <Select
      value={value > 0 ? String(value) : ''}
      onChange={(next) => onChange(Number(next) || 0)}
      options={options}
      placeholder="Choose unlocked difficulty"
      menuGrowToViewport
    />
  </label>
);

interface NumberFieldProps {
  label: string;
  value: number;
  min: number;
  max?: number;
  suffix: string;
  onChange: (value: string) => void;
}

const NumberField: React.FC<NumberFieldProps> = ({ label, value, min, max, suffix, onChange }) => (
  <label className="block">
    <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">{label}</span>
    <Input
      type="number"
      min={min}
      max={max}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      rightIcon={<span className="text-[10px] text-text-muted">{suffix}</span>}
      className="font-mono"
    />
  </label>
);

function activeAdvisorEvent(state: GameStateV2 | null): ScalableEventScoreV2 | null {
  if (!state) return null;
  const preferred = state.eventScores.byEvent[String(state.eventScores.activeEventId ?? 0)];
  if (preferred && supportedAdvisorEvent(preferred.eventId) && (preferred.remainingSec ?? 0) > 0) return preferred;
  return Object.values(state.eventScores.byEvent)
    .filter((score) => supportedAdvisorEvent(score.eventId) && (score.remainingSec ?? 0) > 0)
    .sort((left, right) => Date.parse(right.observedAt) - Date.parse(left.observedAt))[0] ?? null;
}

function supportedAdvisorEvent(eventID: number): boolean {
  return eventID === 72 || eventID === 80;
}

function currencyAmount(state: GameStateV2 | null, currencyID: number): number {
  if (!state || currencyID <= 0) return 0;
  return Math.max(0, Math.trunc(state.player.currencies[String(currencyID)] ?? 0));
}

export default AutoAdvisorSettingsModal;
