import {useLocalizedMessages} from '../../i18n/useLocalizedMessages';
import {parseMessageDescriptor} from '../../i18n/messageDescriptor';
import {messageLanguageAttributes} from '../../i18n/messageLanguage';
import { LocalizedRichText } from "../../i18n/LocalizedRichText";
import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import {
  CalendarDays,
  Castle,
  Clock3,
  Flame,
  Gauge,
  Radar,
  ShoppingBag,
  Snowflake,
  Sun,
  Truck,
  Zap,
} from 'lucide-react';
import UnitImage from '../../components/UnitImage';
import { Badge, Button, Card, Input, SettingsModal, Switch } from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import { DailyAttackLimitField } from './DailyAttackLimitField';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';
import {
  AUTO_FORTRESS_DIREWOLF_ID,
  AUTO_FORTRESS_SECTION,
  clampDirewolfPurchaseLimit,
  defaultAutoFortressClientState,
  parseAutoFortressClientState,
  persistAutoFortressClientState,
  type AutoFortressClientStateV1,
} from '../AutoFortressClientState';

interface AutoFortressSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
}

const KINGDOMS = [
  { id: 1, name: 'Everwinter Glacier', level: 45, icon: Snowflake, tone: 'text-sky-500', wash: 'from-sky-500/15 to-cyan-500/5' },
  { id: 2, name: 'Burning Sands', level: 21, icon: Sun, tone: 'text-amber-500', wash: 'from-amber-500/15 to-orange-500/5' },
  { id: 3, name: 'Fire Peaks', level: 55, icon: Flame, tone: 'text-rose-500', wash: 'from-rose-500/15 to-red-500/5' },
] as const;

const DIREWOLF_TIERS = [
  { range: '1–5,000', rate: '2,340', label: 'Lowest cost' },
  { range: '5,001–10,000', rate: '4,680', label: 'Second tier' },
  { range: '10,001+', rate: '11,700+', label: 'Higher tiers' },
] as const;

function fortressReadyLabel(value: number | undefined): string | null {
  if (!Number.isFinite(value) || Number(value) <= 0) return null;
  return new Date(Number(value) * 1000).toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

export const AutoFortressSettingsModal: React.FC<AutoFortressSettingsModalProps> = ({
  isOpen,
  onClose,
  onOpenFeatureSchedule,
}) => {
  const { t: localizeStatic } = useStaticLocale();
  const { state, configuration } = useCitadelAPI();
  const [settings, setSettings] = useState<AutoFortressClientStateV1>(defaultAutoFortressClientState);
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    if (!isOpen) {
      setSaveError(null);
      return;
    }
    setSettings(parseAutoFortressClientState(configuration?.sections[AUTO_FORTRESS_SECTION]));
  }, [configuration?.sections, isOpen]);

  const castlesByKingdom = useMemo(() => {
    const result = new Map<number, { id: number; name: string; stationed: number }>();
    for (const castle of Object.values(state?.castles ?? {})) {
      if (castle.kingdomId < 1 || castle.kingdomId > 3 || castle.slotType !== 12) continue;
      result.set(castle.kingdomId, {
        id: castle.id,
        name: castle.name?.trim() || `Kingdom ${castle.kingdomId} main castle`,
        stationed: castle.units.stationed[String(AUTO_FORTRESS_DIREWOLF_ID)] ?? 0,
      });
    }
    return result;
  }, [state?.castles]);

  const enabledKingdomCount = Object.values(settings.kingdoms).filter((kingdom) => kingdom.enabled).length;
  const fortressMetrics = state?.automations?.autoFortress?.metrics ?? {};
  const fortressDetails = state?.automations?.autoFortress?.details ?? {};
  const localizedSupplyDetails=useLocalizedMessages(KINGDOMS.map(kingdom=>({legacyText:fortressDetails[`supplyKingdom${kingdom.id}`] ?? '',descriptor:parseMessageDescriptor(state?.automations?.autoFortress?.detailsDescriptors?.[`supplyKingdom${kingdom.id}`])})));
  const nextExpectedReady = fortressReadyLabel(fortressMetrics.nextReadyAtUnix);
  const update = (patch: Partial<AutoFortressClientStateV1>) => setSettings((current) => ({ ...current, ...patch }));

  const toggleKingdom = (kingdomID: number) => {
    const key = String(kingdomID);
    setSettings((current) => ({
      ...current,
      kingdoms: {
        ...current.kingdoms,
        [key]: { enabled: !current.kingdoms[key]?.enabled },
      },
    }));
  };

  const save = async () => {
    if (isSaving) return;
    setIsSaving(true);
    setSaveError(null);
    try {
      await persistAutoFortressClientState(settings);
      onClose();
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Could not save Auto Fortress settings.');
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={() => { if (!isSaving) onClose(); }}
      maxWidth="full"
      title={localizeStatic("ui.settings.components.autoFortressSettingsModal.title.auto.fortress.8b0edaf5")}
      icon={<Castle className="h-5 w-5" />}
      description={localizeStatic("ui.settings.components.autoFortressSettingsModal.description.a.speed.first.fortress.pipeline.discover.a.20cf0ae7")}
      titleTrailing={(
        <Button
          variant="outline"
          size="sm"
          className="shrink-0"
          onClick={() => onOpenFeatureSchedule('autoFortress', 'Auto Fortress')}
          leftIcon={<CalendarDays className="h-4 w-4" />}
        >
          <LocalizedText messageKey="common.calendar" /></Button>
      )}
      onSave={save}
      saveLabel="Save fortress plan"
      isSaving={isSaving}
    >
      {saveError && (
        <div className="mb-4 rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error" role="alert">
          {saveError}
        </div>
      )}

      <div className="mb-4 overflow-hidden rounded-global border border-primary/25 bg-gradient-to-br from-primary/12 via-bg-card to-secondary/8 p-5 shadow-sm">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex min-w-0 items-center gap-4">
            <div className="grid h-16 w-16 shrink-0 place-items-center rounded-2xl border border-primary/25 bg-bg-app/70 shadow-inner">
              <UnitImage unitId={AUTO_FORTRESS_DIREWOLF_ID} size={54} showLevel />
            </div>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-base font-black text-text-main"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.direwolf.blitz.formation.011583d3" /></h3>
                <Badge variant="success"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.1.full.wave.ec16c735" /></Badge>
                <Badge variant="secondary"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.flanks.only.bc229790" /></Badge>
              </div>
              <p className="mt-1 max-w-2xl text-xs leading-relaxed text-text-muted">
                <LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.unit.277.is.filled.across.both.flanks.83af8412" /></p>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-2 text-center sm:grid-cols-3">
            <div className="rounded-xl border border-border-base bg-bg-app/70 px-3 py-2">
              <div className="text-sm font-black text-text-main">24h</div>
              <div className="text-[10px] uppercase tracking-wide text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.global.lock.7cfaa3c9" /></div>
            </div>
            <div className="rounded-xl border border-border-base bg-bg-app/70 px-3 py-2">
              <div className="text-sm font-black text-text-main"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.5.days.4343085b" /></div>
              <div className="text-[10px] uppercase tracking-wide text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.personal.lock.a300b924" /></div>
            </div>
            <div className="col-span-2 rounded-xl border border-border-base bg-bg-app/70 px-3 py-2 sm:col-span-1">
              <div className="text-sm font-black text-text-main">{enabledKingdomCount}/3</div>
              <div className="text-[10px] uppercase tracking-wide text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.kingdoms.armed.98c6dccf" /></div>
            </div>
          </div>
        </div>
      </div>

      <section className="mb-4">
        <div className="mb-2 flex items-center justify-between gap-3">
          <div>
            <h3 className="text-sm font-black text-text-main"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.kingdom.targets.3e092efa" /></h3>
            <p className="mt-0.5 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.each.toggle.uses.that.kingdom.s.main.e96f75b3" /></p>
          </div>
          <div className="flex flex-wrap items-center justify-end gap-2 text-[10px] font-black uppercase tracking-wide text-text-muted">
            <span className="flex items-center gap-1.5 rounded-full border border-border-base bg-bg-app/65 px-2.5 py-1.5">
              <Radar className="h-3.5 w-3.5 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.full.map.discovery.19d8f15c" />
            </span>
            <span className="flex items-center gap-1.5 rounded-full border border-border-base bg-bg-app/65 px-2.5 py-1.5">
              <Clock3 className="h-3.5 w-3.5 text-secondary" /> <LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.due.time.1.1.checks.928cc461" />
            </span>
          </div>
        </div>
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-3">
          {KINGDOMS.map((kingdom,kingdomIndex) => {
            const Icon = kingdom.icon;
            const castle = castlesByKingdom.get(kingdom.id);
            const enabled = settings.kingdoms[String(kingdom.id)]?.enabled === true;
            const knownFortresses = Math.max(0, Math.trunc(fortressMetrics[`knownFortressesKingdom${kingdom.id}`] ?? 0));
            const readyFortresses = Math.max(0, Math.trunc(fortressMetrics[`readyFortressesKingdom${kingdom.id}`] ?? 0));
            const expectedReady = fortressReadyLabel(fortressMetrics[`nextReadyAtKingdom${kingdom.id}Unix`]);
            const stationed = Math.max(0, Math.trunc(fortressMetrics[`stationedDirewolvesKingdom${kingdom.id}`] ?? castle?.stationed ?? 0));
            const inbound = Math.max(0, Math.trunc(fortressMetrics[`inboundDirewolvesKingdom${kingdom.id}`] ?? 0));
            const allocated = Math.max(0, Math.trunc(fortressMetrics[`allocatedDirewolvesKingdom${kingdom.id}`] ?? stationed + inbound));
            const outstanding = Math.max(0, Math.trunc(fortressMetrics[`outstandingDirewolvesKingdom${kingdom.id}`] ?? 0));
            const supplyDetail = localizedSupplyDetails[kingdomIndex];
            return (
              <Card key={kingdom.id} variant="solid" className={`relative overflow-hidden bg-gradient-to-br ${kingdom.wash} p-4`}>
                <div className="flex items-start justify-between gap-3">
                  <div className="flex min-w-0 items-center gap-3">
                    <div className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-border-base bg-bg-app/70">
                      <Icon className={`h-5 w-5 ${kingdom.tone}`} />
                    </div>
                    <div className="min-w-0">
                      <h4 className="truncate text-sm font-black text-text-main">{kingdom.name}</h4>
                      <p className="text-[11px] text-text-muted">Fortress level {kingdom.level}</p>
                    </div>
                  </div>
                  <Switch
                    checked={enabled}
                    disabled={!castle}
                    onChange={() => toggleKingdom(kingdom.id)}
                    ariaLabel={`Toggle Auto Fortress in ${kingdom.name}`}
                  />
                </div>
                <div className="mt-4 rounded-xl border border-border-base bg-bg-app/65 px-3 py-2.5">
                  <div className="truncate text-xs font-bold text-text-main">{castle?.name ?? 'Main castle not detected'}</div>
                  {castle ? (
                    <div className="mt-2 grid grid-cols-2 gap-1.5 text-[10px]">
                      <div className="rounded-lg bg-bg-card/70 px-2 py-1"><span className="block text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.allocated.52d26f44" /></span><strong className="text-text-main">{allocated.toLocaleString()}</strong></div>
                      <div className="rounded-lg bg-bg-card/70 px-2 py-1"><span className="block text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.stationed.81608ba6" /></span><strong className="text-text-main">{stationed.toLocaleString()}</strong></div>
                      <div className="rounded-lg bg-bg-card/70 px-2 py-1"><span className="block text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.inbound.d17a5bdd" /></span><strong className="text-text-main">{inbound.toLocaleString()}</strong></div>
                      <div className="rounded-lg bg-bg-card/70 px-2 py-1"><span className="block text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.outstanding.e681b899" /></span><strong className="text-text-main">{outstanding.toLocaleString()}</strong></div>
                    </div>
                  ) : <div className="mt-1 text-[11px] text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.unlock.kingdom.first.8f49a64e" /></div>}
                  {supplyDetail.text && <div className="mt-2 text-[10px] font-semibold text-text-muted" {...messageLanguageAttributes(supplyDetail)}>{supplyDetail.text}</div>}
                  {castle && (
                    <div className="mt-2 flex items-center gap-1.5 border-t border-border-base/70 pt-2 text-[10px] font-semibold text-text-muted">
                      <Radar className="h-3.5 w-3.5 shrink-0 text-primary" />
                      <span>
                        {knownFortresses === 0
                          ? 'Full-map discovery pending'
                          : readyFortresses > 0
                            ? `${readyFortresses} of ${knownFortresses} tracked fortresses ready`
                            : expectedReady
                              ? `${knownFortresses} tracked · next expected ${expectedReady}`
                              : `${knownFortresses} fortresses tracked`}
                      </span>
                    </div>
                  )}
                </div>
              </Card>
            );
          })}
        </div>
      </section>

      <div className="mb-4 grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Card variant="solid" className="p-4">
          <div className="flex items-start gap-3">
            <div className="grid h-10 w-10 shrink-0 place-items-center rounded-xl bg-primary/10 text-primary"><ShoppingBag className="h-5 w-5" /></div>
            <div className="min-w-0 flex-1">
              <h3 className="text-sm font-black text-text-main"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.nomad.direwolf.supply.651320e5" /></h3>
              <p className="mt-0.5 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.one.exact.per.shop.session.ceiling.purchased.73abffd3" /></p>
            </div>
          </div>
          <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label>
              <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.direwolves.per.session.7d1d3918" /></span>
              <Input
                type="number"
                min={0}
                max={100000}
                step={100}
                value={settings.direwolfPurchaseLimit}
                onChange={(event) => update({ direwolfPurchaseLimit: clampDirewolfPurchaseLimit(event.target.value) })}
                className="font-mono"
                rightIcon={<span className="text-[10px] text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.by.100.1c34dfd3" /></span>}
              />
            </label>
            <label>
              <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.keep.khan.tablets.05e5a18d" /></span>
              <Input
                type="number"
                min={0}
                value={settings.minimumTabletReserve}
                onChange={(event) => update({ minimumTabletReserve: Math.max(0, Math.trunc(Number(event.target.value) || 0)) })}
                className="font-mono"
              />
            </label>
          </div>
          <div className="mt-4 grid grid-cols-1 gap-2 sm:grid-cols-3">
            {DIREWOLF_TIERS.map((tier) => (
              <div key={tier.range} className="rounded-xl border border-border-base bg-bg-app/55 px-3 py-2.5">
                <div className="text-xs font-black text-text-main">{tier.range}</div>
                <div className="mt-0.5 text-[11px] text-text-muted">{tier.rate} tablets / 100</div>
                <div className="mt-1 text-[10px] font-bold uppercase tracking-wide text-primary">{tier.label}</div>
              </div>
            ))}
          </div>
          <div className="mt-3 flex items-start gap-2 rounded-xl border border-secondary/20 bg-secondary/5 px-3 py-2.5 text-[11px] text-text-muted">
            <Truck className="mt-0.5 h-4 w-4 shrink-0 text-secondary" />
            <LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.purchases.arrive.at.the.great.empire.main.e7405a14" />
          </div>
          <div className="mt-3 flex items-center justify-between gap-4 rounded-xl border border-border-base bg-bg-app/55 px-3 py-3">
            <div>
              <div className="text-xs font-black text-text-main"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.use.time.skips.for.direwolf.transfers.e1a00ae9" /></div>
              <p className="mt-0.5 text-[11px] text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.off.by.default.when.enabled.only.confirmed.51b3f8c5" /></p>
            </div>
            <Switch checked={settings.useTimeSkips} onChange={() => update({ useTimeSkips: !settings.useTimeSkips })} ariaLabel={localizeStatic("ui.settings.components.autoFortressSettingsModal.ariaLabel.use.time.skips.for.direwolf.transfers.e1a00ae9")} />
          </div>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="flex items-start gap-3">
            <div className="grid h-10 w-10 shrink-0 place-items-center rounded-xl bg-amber-500/10 text-amber-500"><Zap className="h-5 w-5" /></div>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-sm font-black text-text-main"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.march.speed.b518df50" /></h3>
                <Badge variant="success"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.relic.2.0.required.8de8ec76" /></Badge>
              </div>
              <p className="mt-0.5 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.auto.fortress.always.selects.the.full.100.0058f20b" /></p>
            </div>
          </div>
          <div className="mt-4 flex items-start gap-2 rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2.5 text-[11px] text-text-muted">
            <Zap className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
            <span><LocalizedRichText messageKey="ui.rich.settings.components.autoFortressSettingsModal.recommended.enable.the.separate.auto.booster.feature.13180338" params={{cost:2500}} tags={{strong0: children => <strong className="text-text-main">{children}</strong>}} /></span>
          </div>
          <div className="mt-4 rounded-xl border border-border-base bg-bg-app/55 p-3">
            <HorseTravelBoostSelect
              value={settings.horseTravelBoostId}
              onChange={(horseTravelBoostId) => update({ horseTravelBoostId })}
              description={localizeStatic("ui.settings.components.autoFortressSettingsModal.description.courser.fastest.tier.is.the.default.its.fe582b66")}
            />
          </div>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <DailyAttackLimitField value={settings.dailyAttackLimit} onChange={(dailyAttackLimit) => update({ dailyAttackLimit })} serverState={state?.dailyAttacks} />
        <div className="flex items-center gap-3 rounded-global border border-border-base bg-bg-card/50 p-4">
          <Gauge className="h-5 w-5 shrink-0 text-primary" />
          <div>
            <div className="text-xs font-black text-text-main"><LocalizedText messageKey="ui.settings.components.autoFortressSettingsModal.full.map.cache.exact.ready.time.checks.b3e0112d" /></div>
            <p className="mt-0.5 text-[11px] text-text-muted">
              Adaptive sweeps discover every populated map chunk. The account-private timer then schedules a 1×1 refresh at availability and another immediate guard before CRA.
              {nextExpectedReady ? ` Earliest tracked availability: ${nextExpectedReady}.` : ''}
            </p>
          </div>
        </div>
      </div>
    </SettingsModal>
  );
};
