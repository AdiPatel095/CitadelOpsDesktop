import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import { CalendarDays, Clock3, Coins, ShieldCheck, Sparkles, Zap } from 'lucide-react';
import { useCitadelAPI } from '../../api/ApiContext';
import { Badge, Button, Card, Input, SettingsModal } from '../../components/ui';
import {
  AUTO_BOOSTER_RUBY_COST,
  AUTO_BOOSTER_SECTION,
  defaultAutoBoosterClientState,
  parseAutoBoosterClientState,
  persistAutoBoosterClientState,
  type AutoBoosterClientStateV1,
} from '../AutoBoosterClientState';
import {
  deriveAutoBoosterViewState,
  formatAutoBoosterRemaining,
  formatAutoBoosterRequestProgress,
  formatObservedRubyChange,
  hasMeaningfulAutoBoosterTime,
} from '../AutoBoosterViewState';

interface AutoBoosterSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
}

function formatReceiptTime(value: string | undefined): string {
  if (!value) return 'Not observed';
  const date = new Date(value);
  if (!hasMeaningfulAutoBoosterTime(value)) return 'Not observed';
  return date.toLocaleString([], { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', second: '2-digit' });
}

export const AutoBoosterSettingsModal: React.FC<AutoBoosterSettingsModalProps> = ({
  isOpen,
  onClose,
  onOpenFeatureSchedule,
}) => {
  const { state, configuration } = useCitadelAPI();
  const [settings, setSettings] = useState<AutoBoosterClientStateV1>(defaultAutoBoosterClientState);
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!isOpen) {
      setSaveError(null);
      return;
    }
    setSettings(parseAutoBoosterClientState(configuration?.sections[AUTO_BOOSTER_SECTION]));
  }, [configuration?.sections, isOpen]);

  useEffect(() => {
    if (!isOpen) return undefined;
    setNow(Date.now());
    const interval = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(interval);
  }, [isOpen]);

  const live = useMemo(() => deriveAutoBoosterViewState(
    state?.eventScores.inventory,
    now,
    state?.automations.autoBooster?.detail,
  ), [now, state?.automations.autoBooster?.detail, state?.eventScores.inventory]);

  const statusVariant = live.status === 'active'
    ? 'success'
    : live.status === 'rejected'
      ? 'danger'
      : live.status === 'accepted' || live.status === 'unresolved'
        ? 'warning'
        : 'outline';

  const save = async () => {
    if (isSaving) return;
    setIsSaving(true);
    setSaveError(null);
    try {
      await persistAutoBoosterClientState(settings);
      onClose();
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Could not save Auto Booster settings.');
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={() => { if (!isSaving) onClose(); }}
      maxWidth="lg"
      title="Auto Booster"
      icon={<Zap className="h-5 w-5" />}
      description="A standalone daily purchase lane for the premium global fortress-speed boost. It never controls or blocks Auto Fortress."
      titleTrailing={(
        <Button
          variant="outline"
          size="sm"
          className="shrink-0"
          onClick={() => onOpenFeatureSchedule('autoBooster', 'Auto Booster')}
          leftIcon={<CalendarDays className="h-4 w-4" />}
        >
          <LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.calendar.d5d0a30b" /></Button>
      )}
      onSave={save}
      saveLabel="Save booster guard"
      isSaving={isSaving}
    >
      {saveError && (
        <div className="mb-4 rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error" role="alert">
          {saveError}
        </div>
      )}

      <div className="mb-4 overflow-hidden rounded-global border border-amber-500/30 bg-gradient-to-br from-amber-500/15 via-bg-card to-primary/8 p-5 shadow-sm">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex min-w-0 items-center gap-4">
            <div className="grid h-14 w-14 shrink-0 place-items-center rounded-2xl border border-amber-500/30 bg-bg-app/70 text-amber-500 shadow-inner">
              <Sparkles className="h-7 w-7" />
            </div>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-base font-black text-text-main"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.daily.fortress.speed.boost.2fb815fe" /></h3>
                <Badge variant="warning"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.2.500.rubies.966ac160" /></Badge>
                <Badge variant={statusVariant}>{live.statusLabel}</Badge>
              </div>
              <p className="mt-1 max-w-xl text-xs leading-relaxed text-text-muted">
                {live.statusDetail}
              </p>
            </div>
          </div>
          <div className="rounded-xl border border-border-base bg-bg-app/70 px-3 py-2 text-right">
            <div className="text-xs font-black text-text-main">
              {live.offer ? `${live.offer.rubyCost.toLocaleString()} rubies quoted` : 'No live quote yet'}
            </div>
            <div className="mt-0.5 text-[10px] uppercase tracking-wide text-text-muted">
              {formatAutoBoosterRemaining(live.expiresAt, now)}
            </div>
          </div>
        </div>
      </div>

      {live.purchase && (
        <Card variant="solid" className="mb-4 p-4" data-testid="auto-booster-purchase-record">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 className="text-sm font-black text-text-main">{live.purchaseHeading}</h3>
              <p className="mt-1 text-xs text-text-muted">{live.purchase.detail ?? 'Waiting for purchase evidence from the game.'}</p>
            </div>
            <Badge variant={live.purchaseIsCurrent && live.purchase.outcome === 'confirmed' ? 'success' : live.purchaseIsCurrent && live.purchase.outcome === 'rejected' ? 'danger' : 'warning'}>
              {!live.purchaseIsCurrent ? 'Historical' : live.purchase.outcome === 'confirmed' ? 'Covered' : live.purchase.outcome === 'accepted' ? 'Accepted' : live.purchase.outcome === 'unresolved' ? 'Unresolved' : 'Rejected'}
            </Badge>
          </div>
          <div className="mt-3 grid grid-cols-2 gap-2 text-[11px] md:grid-cols-4">
            <div className="rounded-xl border border-border-base bg-bg-app/55 px-3 py-2"><span className="block text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.event.expiry.cfd4614e" /></span><strong className="text-text-main">{formatAutoBoosterRemaining(live.purchase.expiresAt, now)}</strong><span className="mt-0.5 block text-[10px] text-text-muted">{formatReceiptTime(live.purchase.expiresAt)}</span></div>
            <div className="rounded-xl border border-border-base bg-bg-app/55 px-3 py-2"><span className="block text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.quote.and.reserve.a6203198" /></span><strong className="text-text-main">{live.purchaseHasRequest ? `${live.purchase.quotedRubyCost.toLocaleString()} rubies` : 'No purchase quote'}</strong><span className="mt-0.5 block text-[10px] text-text-muted">{live.purchaseHasRequest ? `Keep ${live.purchase.minimumRubyReserve.toLocaleString()}` : 'Reserve not recorded'}</span></div>
            <div className="rounded-xl border border-border-base bg-bg-app/55 px-3 py-2"><span className="block text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.request.and.result.2eb7af2b" /></span><strong className="text-text-main">{live.purchaseHasRequest ? `${live.purchase.requestOpcode.toUpperCase()} · ${live.purchase.resultCode == null ? 'Awaiting result' : `Code ${live.purchase.resultCode}`}` : 'No automated request'}</strong><span className="mt-0.5 block text-[10px] text-text-muted">{live.purchaseHasRequest ? formatAutoBoosterRequestProgress(live.purchase) : 'Activation observed from game state'}</span></div>
            <div className="rounded-xl border border-border-base bg-bg-app/55 px-3 py-2"><span className="block text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.ruby.observation.9c1604b5" /></span><strong className="text-text-main">{live.purchaseHasRequest ? formatObservedRubyChange(live.purchase) : 'No purchase balance evidence'}</strong>{live.purchaseHasRequest && <span className="mt-0.5 block text-[10px] text-text-muted">{live.purchase.rubyBefore.toLocaleString()} before{live.purchase.rubyAfterKnown ? ` · ${(live.purchase.rubyAfter ?? 0).toLocaleString()} after` : ''}</span>}<span className="mt-0.5 block text-[10px] font-semibold text-text-main">{live.purchase.debitUnverified ? 'Purchase debit unverified' : 'Purchase debit verified'}</span></div>
          </div>
          <details className="mt-3 rounded-xl border border-border-base bg-bg-app/40 px-3 py-2 text-[11px] text-text-muted">
            <summary className="cursor-pointer font-bold text-text-main"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.receipt.details.02f7cc37" /></summary>
            <dl className="mt-2 grid grid-cols-1 gap-x-4 gap-y-2 sm:grid-cols-2">
              <div><dt><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.request.prepared.8e687b36" /></dt><dd className="font-mono text-text-main">{live.purchaseHasRequest ? formatReceiptTime(live.purchase.requestedAt) : 'Unavailable'}</dd></div>
              <div><dt><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.request.dispatched.ce6eb892" /></dt><dd className="font-mono text-text-main">{live.purchaseHasRequest ? formatReceiptTime(live.purchase.dispatchedAt) : 'Unavailable'}</dd></div>
              <div><dt><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.acknowledgement.observed.469bfdf6" /></dt><dd className="font-mono text-text-main">{live.purchaseHasRequest ? formatReceiptTime(live.purchase.resultObservedAt) : 'Unavailable'}</dd></div>
              <div><dt><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.activation.observed.a9cb1031" /></dt><dd className="font-mono text-text-main">{formatReceiptTime(live.purchase.activationObservedAt)}</dd></div>
              <div><dt><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.quote.bonus.9d62dae4" /></dt><dd className="font-mono text-text-main">{live.purchaseHasRequest ? live.purchase.quotedBonusValue.toLocaleString() : 'Unavailable'}</dd></div>
              <div><dt><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.operation.0f044feb" /></dt><dd className="break-all font-mono text-text-main">{live.purchase.operationId || 'Unavailable'}</dd></div>
            </dl>
          </details>
        </Card>
      )}

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Card variant="solid" className="p-4">
          <div className="flex items-start gap-3">
            <div className="grid h-10 w-10 shrink-0 place-items-center rounded-xl bg-primary/10 text-primary">
              <Coins className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-sm font-black text-text-main"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.ruby.reserve.bd9dd746" /></h3>
              <p className="mt-0.5 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.the.purchase.must.leave.at.least.this.a3c95609" /></p>
            </div>
          </div>
          <label className="mt-4 block">
            <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.minimum.rubies.to.keep.d6f81306" /></span>
            <Input
              type="number"
              min={0}
              value={settings.minimumRubyReserve}
              onChange={(event) => setSettings((current) => ({
                ...current,
                minimumRubyReserve: Math.max(0, Math.trunc(Number(event.target.value) || 0)),
              }))}
              className="font-mono"
            />
          </label>
          <div className="mt-3 rounded-xl border border-border-base bg-bg-app/55 px-3 py-2.5 text-[11px] text-text-muted">
            Fixed spend ceiling: <strong className="text-text-main">{AUTO_BOOSTER_RUBY_COST.toLocaleString()} rubies</strong>. A different live price is rejected, even when the balance is sufficient.
          </div>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="flex items-start gap-3">
            <div className="grid h-10 w-10 shrink-0 place-items-center rounded-xl bg-secondary/10 text-secondary">
              <ShieldCheck className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-sm font-black text-text-main"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.dispatch.safeguards.249ed03e" /></h3>
              <p className="mt-0.5 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.all.checks.are.repeated.immediately.before.premium.2f36dd30" /></p>
            </div>
          </div>
          <div className="mt-4 space-y-2 text-[11px] text-text-muted">
            <div className="flex items-start gap-2 rounded-xl border border-border-base bg-bg-app/55 px-3 py-2.5">
              <Clock3 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-primary" />
              The daily effect window and its exact end time must still match.
            </div>
            <div className="flex items-start gap-2 rounded-xl border border-border-base bg-bg-app/55 px-3 py-2.5">
              <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0 text-secondary" />
              The server must report effect 2 as not yet boosted in this same window.
            </div>
            <div className="flex items-start gap-2 rounded-xl border border-border-base bg-bg-app/55 px-3 py-2.5">
              <Coins className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-500" />
              The quote must remain exactly 2,500 and the current balance must preserve your reserve.
            </div>
          </div>
        </Card>
      </div>

      <div className="mt-4 flex items-start gap-3 rounded-global border border-primary/25 bg-primary/5 p-4 text-xs text-text-muted">
        <Zap className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
        <p><strong className="text-text-main"><LocalizedText messageKey="ui.settings.components.autoBoosterSettingsModal.independent.by.design.c461f898" /></strong> Auto Booster only buys this one daily global effect. Auto Fortress can run without it, while enabling both is recommended for the fastest fortress marches.</p>
      </div>
    </SettingsModal>
  );
};
