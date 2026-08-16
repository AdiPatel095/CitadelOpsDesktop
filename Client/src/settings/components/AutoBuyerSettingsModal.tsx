import React, { useEffect, useMemo, useState } from 'react';
import { Clock3, Coins, PackageSearch, ShieldCheck, ShoppingCart, Sparkles, Store, Users } from 'lucide-react';
import { CitadelAPI } from '../../api/CitadelClient';
import { useCitadelAPI } from '../../api/ApiContext';
import { castleOptionsFromState } from '../../api/Selectors';
import type {
  AutoBuyerFeastV1,
  AutoBuyerPackageV1,
  AutoBuyerProjectionV1,
  AutoBuyerSpecialistV1,
} from '../../api/Contracts';
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Card, Input, Select, SettingsModal, Switch } from '../../components/ui';
import {
  AUTO_BUYER_MINIMUM_SPECIALIST_DAYS,
  AUTO_BUYER_SECTION,
  clampAutoBuyerInteger,
  defaultAutoBuyerClientState,
  parseAutoBuyerClientState,
  type AutoBuyerClientStateV1,
  type AutoBuyerPackageRuleV1,
  type AutoBuyerSpecialistRuleV1,
} from '../AutoBuyerClientState';

interface AutoBuyerSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

type AutoBuyerSection = 'shops' | 'specialists' | 'feast';
const ALL_AUTO_BUYER_CURRENCIES = 'all';

export const AutoBuyerSettingsModal: React.FC<AutoBuyerSettingsModalProps> = ({ isOpen, onClose }) => {
  const { state, configuration, updateConfiguration } = useCitadelAPI();
  const [draft, setDraft] = useState<AutoBuyerClientStateV1>(defaultAutoBuyerClientState);
  const [projection, setProjection] = useState<AutoBuyerProjectionV1 | null>(null);
  const [loadError, setLoadError] = useState('');
  const [saving, setSaving] = useState(false);
  const [section, setSection] = useState<AutoBuyerSection>('shops');
  const [selectedShopId, setSelectedShopId] = useState('');
  const [selectedCurrencyKey, setSelectedCurrencyKey] = useState(ALL_AUTO_BUYER_CURRENCIES);
  const [query, setQuery] = useState('');
  const castles = useMemo(
    () => castleOptionsFromState(state).filter((castle) => castle.kingdomId === 0 && castle.type === 'Slot 1'),
    [state],
  );
  const defaultCastleID = castles[0]?.id ?? 0;

  useEffect(() => {
    if (!isOpen) return;
    const parsed = parseAutoBuyerClientState(configuration?.sections[AUTO_BUYER_SECTION]);
    setDraft({
      ...parsed,
      sourceCastleId: parsed.sourceCastleId || defaultCastleID,
      feast: { ...parsed.feast, sourceCastleId: parsed.feast.sourceCastleId || parsed.sourceCastleId || defaultCastleID },
    });
    setSection('shops');
    setSelectedShopId(parsed.packages.find((rule) => rule.enabled)?.shopId ?? '');
    setSelectedCurrencyKey(ALL_AUTO_BUYER_CURRENCIES);
    setQuery('');
  }, [configuration?.sections, defaultCastleID, isOpen]);

  useEffect(() => {
    if (!isOpen) return;
    let cancelled = false;
    setProjection(null);
    setLoadError('');
    void CitadelAPI.getProjection<AutoBuyerProjectionV1>('auto-buyer')
      .then((catalog) => {
        if (cancelled) return;
        setProjection(catalog);
        setSelectedShopId((current) => (
          catalog.shops.some((shop) => shop.id === current) ? current : (catalog.shops[0]?.id ?? '')
        ));
        setDraft((current) => {
          const feastExists = catalog.feasts.some((feast) => feast.id === current.feast.feastId);
          return feastExists || catalog.feasts.length === 0
            ? current
            : { ...current, feast: { ...current.feast, feastId: catalog.feasts[0].id } };
        });
      })
      .catch((error) => {
        if (!cancelled) setLoadError(error instanceof Error ? error.message : 'Could not load the official Auto Buyer catalog.');
      });
    return () => { cancelled = true; };
  }, [isOpen]);

  const packageRules = useMemo(
    () => new Map(draft.packages.map((rule) => [`${rule.shopId}:${rule.packageId}`, rule])),
    [draft.packages],
  );
  const specialistRules = useMemo(
    () => new Map(draft.specialists.map((rule) => [rule.id, rule])),
    [draft.specialists],
  );
  const selectedFeast = projection?.feasts.find((feast) => feast.id === draft.feast.feastId) ?? null;
  const selectedShop = projection?.shops.find((shop) => shop.id === selectedShopId) ?? null;
  const currencyOptions = useMemo(() => {
    if (!projection || !selectedShopId) return [];
    const currencies = new Map<string, { value: string; name: string; count: number; premium: boolean }>();
    for (const product of projection.packages) {
      if (product.shopId !== selectedShopId) continue;
      const value = autoBuyerCurrencyKey(product);
      const existing = currencies.get(value);
      if (existing) {
        existing.count += 1;
      } else {
        currencies.set(value, { value, name: product.price.name, count: 1, premium: product.price.premium });
      }
    }
    return [...currencies.values()].sort((left, right) => (
      Number(left.premium) - Number(right.premium) || left.name.localeCompare(right.name)
    ));
  }, [projection, selectedShopId]);
  const filteredPackages = useMemo(() => {
    if (!projection || !selectedShopId) return [];
    const shopPackages = projection.packages.filter((product) => (
      product.shopId === selectedShopId && (
        selectedCurrencyKey === ALL_AUTO_BUYER_CURRENCIES || autoBuyerCurrencyKey(product) === selectedCurrencyKey
      )
    ));
    const normalized = query.trim().toLowerCase();
    if (!normalized) return shopPackages;
    return shopPackages.filter((product) => (
      `${product.name} ${product.detail ?? ''} ${product.packageId}`.toLowerCase().includes(normalized)
    ));
  }, [projection, query, selectedCurrencyKey, selectedShopId]);
  const enabledPackageCountByShop = useMemo(() => {
    const counts = new Map<string, number>();
    for (const rule of draft.packages) {
      if (rule.enabled) counts.set(rule.shopId, (counts.get(rule.shopId) ?? 0) + 1);
    }
    return counts;
  }, [draft.packages]);

  const updatePackage = (product: AutoBuyerPackageV1, update: Partial<AutoBuyerPackageRuleV1>) => {
    setDraft((current) => {
      const key = `${product.shopId}:${product.packageId}`;
      const existing = current.packages.find((rule) => `${rule.shopId}:${rule.packageId}` === key) ?? {
        enabled: false,
        shopId: product.shopId,
        packageId: product.packageId,
        targetPurchasesPerReset: 1,
        minimumBalanceReserve: 0,
        maximumRubySpendPerReset: product.price.premium ? product.price.amount : 0,
      };
      const next = { ...existing, ...update };
      const present = current.packages.some((rule) => `${rule.shopId}:${rule.packageId}` === key);
      return {
        ...current,
        packages: present
          ? current.packages.map((rule) => (`${rule.shopId}:${rule.packageId}` === key ? next : rule))
          : [...current.packages, next],
      };
    });
  };

  const updateSpecialist = (specialist: AutoBuyerSpecialistV1, update: Partial<AutoBuyerSpecialistRuleV1>) => {
    setDraft((current) => {
      const existing = current.specialists.find((rule) => rule.id === specialist.id) ?? {
        enabled: false,
        id: specialist.id,
        minimumDays: AUTO_BUYER_MINIMUM_SPECIALIST_DAYS,
        maximumRubyCostPerPurchase: specialist.baseRubyCost,
      };
      const next = { ...existing, ...update };
      const present = current.specialists.some((rule) => rule.id === specialist.id);
      return {
        ...current,
        specialists: present
          ? current.specialists.map((rule) => (rule.id === specialist.id ? next : rule))
          : [...current.specialists, next],
      };
    });
  };

  const setAllSpecialists = (enabled: boolean) => {
    if (!projection) return;
    setDraft((current) => ({
      ...current,
      specialists: projection.specialists.map((specialist) => {
        const existing = current.specialists.find((rule) => rule.id === specialist.id);
        return {
          enabled,
          id: specialist.id,
          minimumDays: Math.max(AUTO_BUYER_MINIMUM_SPECIALIST_DAYS, existing?.minimumDays ?? AUTO_BUYER_MINIMUM_SPECIALIST_DAYS),
          maximumRubyCostPerPurchase: Math.max(specialist.baseRubyCost, existing?.maximumRubyCostPerPurchase ?? 0),
        };
      }),
    }));
  };

  const enabledPackages = draft.packages.filter((rule) => rule.enabled);
  const enabledSpecialists = draft.specialists.filter((rule) => rule.enabled);
  const configurationValid = useMemo(() => {
    if (!projection) return false;
    if ((enabledPackages.length > 0 || draft.feast.enabled) && draft.sourceCastleId <= 0) return false;
    for (const rule of enabledPackages) {
      const product = projection.packages.find((candidate) => candidate.shopId === rule.shopId && candidate.packageId === rule.packageId);
      if (!product || rule.targetPurchasesPerReset < 1 || rule.targetPurchasesPerReset > product.stock) return false;
      if (product.price.premium && (!draft.allowRubyPackages || rule.maximumRubySpendPerReset < product.price.amount)) return false;
    }
    for (const rule of enabledSpecialists) {
      const specialist = projection.specialists.find((candidate) => candidate.id === rule.id);
      if (!specialist || rule.minimumDays < AUTO_BUYER_MINIMUM_SPECIALIST_DAYS || rule.maximumRubyCostPerPurchase < specialist.baseRubyCost) return false;
    }
    if (draft.feast.enabled) {
      if (!selectedFeast || (draft.feast.sourceCastleId || draft.sourceCastleId) <= 0 || draft.feast.minimumRemainingHours < 1) return false;
      if (selectedFeast.price.premium && (!draft.feast.allowRubies || draft.feast.maximumRubyCostPerPurchase < selectedFeast.price.amount)) return false;
    }
    return true;
  }, [draft, enabledPackages, enabledSpecialists, projection, selectedFeast]);

  const save = async () => {
    if (saving || !configurationValid) return;
    setSaving(true);
    try {
      const normalized = parseAutoBuyerClientState({
        ...draft,
        feast: { ...draft.feast, sourceCastleId: draft.feast.sourceCastleId || draft.sourceCastleId },
      });
      await updateConfiguration(AUTO_BUYER_SECTION, normalized);
      Notifications.success('Auto Buyer settings saved.');
      onClose();
    } catch (error) {
      Notifications.error(error instanceof Error ? error.message : 'Could not save Auto Buyer settings.');
    } finally {
      setSaving(false);
    }
  };

  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={() => { if (!saving) onClose(); }}
      maxWidth="6xl"
      title="Auto Buyer"
      icon={<ShoppingCart className="h-5 w-5" />}
      description="Stock resets, specialist floors, and feast upkeep with hard spending guards"
      onSave={() => void save()}
      isSaving={saving}
      saveDisabled={!configurationValid || Boolean(loadError)}
    >
      <div className="space-y-3">
        <Card variant="solid" className="p-4">
          <div className="mb-4 flex items-start gap-3">
            <span className="rounded-xl bg-primary/10 p-2 text-primary"><ShieldCheck className="h-5 w-5" /></span>
            <div>
              <h3 className="text-sm font-black text-text-main">Account-wide safety limits</h3>
              <p className="mt-1 text-xs text-text-muted">Auto Buyer sends one bounded operation at a time, rechecks live state before spending, and verifies the server counter or timer afterward.</p>
            </div>
          </div>
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <label className="block xl:col-span-2">
              <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Great Empire main castle</span>
              <Select
                value={draft.sourceCastleId > 0 ? String(draft.sourceCastleId) : ''}
                onChange={(value) => setDraft((current) => ({ ...current, sourceCastleId: Number(value) || 0 }))}
                options={castles.map((castle) => ({ value: String(castle.id), label: `${castle.name} · ${castle.x}:${castle.y}` }))}
                placeholder="Choose the main castle"
                menuGrowToViewport
              />
            </label>
			<NumberField
			  label="Check every (minutes)"
			  value={Math.round(draft.checkIntervalSec / 60)}
			  minimum={30}
			  maximum={60}
			  onChange={(minutes) => setDraft((current) => ({ ...current, checkIntervalSec: minutes * 60 }))}
			/>
            <NumberField
              label="Keep at least rubies"
              value={draft.minimumRubyReserve}
              minimum={0}
              onChange={(minimumRubyReserve) => setDraft((current) => ({ ...current, minimumRubyReserve }))}
            />
          </div>
          <div className="mt-4 flex items-center justify-between gap-4 border-t border-border-base pt-4">
            <div>
              <div className="text-sm font-bold text-text-main">Allow ruby-priced shop packages</div>
              <p className="mt-0.5 text-xs text-text-muted">Each package still needs its own per-reset ruby ceiling. Specialists and ruby feasts use separate ceilings.</p>
            </div>
            <Switch
              checked={draft.allowRubyPackages}
              onChange={(allowRubyPackages) => setDraft((current) => ({ ...current, allowRubyPackages }))}
              ariaLabel="Allow ruby-priced shop packages"
            />
          </div>
        </Card>

        <div className="flex flex-wrap gap-2">
          <SectionButton active={section === 'shops'} onClick={() => setSection('shops')} icon={<Store className="h-4 w-4" />}>
            Shops <Badge variant="outline">{enabledPackages.length}</Badge>
          </SectionButton>
          <SectionButton active={section === 'specialists'} onClick={() => setSection('specialists')} icon={<Users className="h-4 w-4" />}>
            Specialists <Badge variant="outline">{enabledSpecialists.length}</Badge>
          </SectionButton>
          <SectionButton active={section === 'feast'} onClick={() => setSection('feast')} icon={<Sparkles className="h-4 w-4" />}>
            Feast {draft.feast.enabled ? <Badge variant="success">On</Badge> : null}
          </SectionButton>
        </div>

        {loadError ? (
          <Card variant="solid" className="border-error/40 p-4 text-sm text-error">{loadError}</Card>
        ) : !projection ? (
          <Card variant="solid" className="p-8 text-center text-sm text-text-muted">Loading the current official purchase catalog…</Card>
        ) : null}

        {projection && section === 'shops' ? (
          <div className="space-y-3">
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(0,18rem)_minmax(0,18rem)_minmax(0,1fr)]">
              <label className="block">
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Shop</span>
                <Select
                  value={selectedShopId}
                  onChange={(value) => {
                    setSelectedShopId(value);
                    setSelectedCurrencyKey(ALL_AUTO_BUYER_CURRENCIES);
                    setQuery('');
                  }}
                  options={projection.shops.map((shop) => {
                    const selectedCount = enabledPackageCountByShop.get(shop.id) ?? 0;
                    return {
                      value: shop.id,
                      label: `${shop.name} · ${shop.packageCount} item${shop.packageCount === 1 ? '' : 's'}${selectedCount > 0 ? ` · ${selectedCount} selected` : ''}`,
                    };
                  })}
                  placeholder="Choose a supported shop"
                  icon={<Store className="h-4 w-4" />}
                  ariaLabel="Auto Buyer shop"
                  menuGrowToViewport
                />
              </label>
              <label className="block">
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Currency</span>
                <Select
                  value={selectedCurrencyKey}
                  onChange={setSelectedCurrencyKey}
                  options={[
                    {
                      value: ALL_AUTO_BUYER_CURRENCIES,
                      label: `All currencies · ${selectedShop?.packageCount ?? 0} items`,
                    },
                    ...currencyOptions.map((currency) => ({
                      value: currency.value,
                      label: `${currency.name} · ${currency.count} item${currency.count === 1 ? '' : 's'}`,
                    })),
                  ]}
                  placeholder="Choose a currency"
                  icon={<Coins className="h-4 w-4" />}
                  ariaLabel="Auto Buyer currency"
                  menuGrowToViewport
                />
              </label>
              <label className="block md:col-span-2 xl:col-span-1">
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Search selected shop</span>
                <Input
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder={`Search ${selectedShop?.name ?? 'shop'} items or package IDs`}
                  leftIcon={<PackageSearch className="h-4 w-4" />}
                />
              </label>
            </div>
            {selectedShop ? (
              <Card key={selectedShop.id} variant="solid" className="overflow-hidden">
                <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border-base px-4 py-3">
                  <div className="flex items-center gap-2">
                    <h3 className="text-sm font-black text-text-main">{selectedShop.name}</h3>
                    <Badge variant={selectedShop.requiresEvent ? 'warning' : 'outline'}>{selectedShop.requiresEvent ? 'Event only' : 'Reset tracked'}</Badge>
                    {(enabledPackageCountByShop.get(selectedShop.id) ?? 0) > 0 ? (
                      <Badge variant="secondary">{enabledPackageCountByShop.get(selectedShop.id)} selected</Badge>
                    ) : null}
                  </div>
                  <span className="text-xs text-text-muted">
                    {filteredPackages.length}{query.trim() || selectedCurrencyKey !== ALL_AUTO_BUYER_CURRENCIES ? ` of ${selectedShop.packageCount}` : ''} supported item{filteredPackages.length === 1 ? '' : 's'}
                  </span>
                </div>
                <div className="divide-y divide-border-base">
                  {filteredPackages.map((product) => {
                    const rule = packageRules.get(`${product.shopId}:${product.packageId}`);
                    const enabled = rule?.enabled === true;
                    const purchased = state?.inventory.constructionOffers[String(product.packageId)] ?? 0;
                    return (
                      <div key={`${product.shopId}:${product.packageId}`} className="p-4">
                        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                          <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="font-bold text-text-main">{product.name}</span>
                              <Badge variant="outline">#{product.packageId}</Badge>
                              <Badge variant={product.price.premium ? 'warning' : 'secondary'}>{formatPrice(product)}</Badge>
                              <Badge variant="outline">{purchased}/{product.stock} bought</Badge>
                            </div>
                            {product.detail ? <p className="mt-1 text-xs text-text-muted">{product.detail}</p> : null}
                          </div>
                          <div className="flex shrink-0 items-end gap-3">
                            <div className="w-44">
                              <NumberField
                                label="Purchase limit / reset"
                                value={rule?.targetPurchasesPerReset ?? 1}
                                minimum={1}
                                maximum={product.stock}
                                onChange={(targetPurchasesPerReset) => updatePackage(product, { targetPurchasesPerReset })}
                              />
                            </div>
                            <Switch
                              checked={enabled}
                              onChange={(value) => updatePackage(product, {
                                enabled: value,
                                maximumRubySpendPerReset: product.price.premium
                                  ? Math.max(rule?.maximumRubySpendPerReset ?? 0, product.price.amount)
                                  : 0,
                              })}
                              ariaLabel={`Automatically buy ${product.name}`}
                            />
                          </div>
                        </div>
                        {enabled ? (
                          <div className="mt-3 grid gap-3 border-t border-border-base pt-3 md:grid-cols-2">
                            <NumberField
                              label={`Keep at least ${product.price.name}`}
                              value={rule?.minimumBalanceReserve ?? 0}
                              minimum={0}
                              onChange={(minimumBalanceReserve) => updatePackage(product, { minimumBalanceReserve })}
                            />
                            {product.price.premium ? (
                              <NumberField
                                label="Max rubies per stock reset"
                                value={rule?.maximumRubySpendPerReset ?? product.price.amount}
                                minimum={product.price.amount}
                                onChange={(maximumRubySpendPerReset) => updatePackage(product, { maximumRubySpendPerReset })}
                              />
                            ) : (
                              <div className="rounded-xl border border-border-base bg-surface-base/40 px-3 py-2 text-xs text-text-muted">
                                Auto Buyer stops at the purchase limit, then waits for the server stock counter to reset.
                              </div>
                            )}
                          </div>
                        ) : null}
                      </div>
                    );
                  })}
                  {filteredPackages.length === 0 ? (
                    <div className="p-8 text-center text-sm text-text-muted">No supported stock-limited items match these filters.</div>
                  ) : null}
                </div>
              </Card>
            ) : (
              <Card variant="solid" className="p-8 text-center text-sm text-text-muted">No supported shops are available in the current official catalog.</Card>
            )}
          </div>
        ) : null}

        {projection && section === 'specialists' ? (
          <Card variant="solid" className="overflow-hidden">
            <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border-base p-4">
              <div>
                <h3 className="text-sm font-black text-text-main">Specialist renewal floors</h3>
                <p className="mt-1 text-xs text-text-muted">Enabled floors are clamped to at least 14 days. Renewals happen one 7-day purchase at a time so the active rebuy discount remains eligible.</p>
              </div>
              <div className="flex gap-2">
                <Button variant="ghost" onClick={() => setAllSpecialists(false)}>Disable all</Button>
                <Button variant="secondary" onClick={() => setAllSpecialists(true)}>Enable all at 14 days</Button>
              </div>
            </div>
            <div className="divide-y divide-border-base">
              {projection.specialists.map((specialist) => {
                const rule = specialistRules.get(specialist.id);
                const enabled = rule?.enabled === true;
                const current = state?.market.boosters?.[String(specialist.id)];
                return (
                  <div key={specialist.id} className="p-4">
                    <div className="flex items-start justify-between gap-4">
                      <div>
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-bold text-text-main">{specialist.name}</span>
                          {specialist.bonusPercent ? <Badge variant="secondary">+{specialist.bonusPercent}%</Badge> : null}
                          <Badge variant="outline">{formatRemaining(current?.expiresAt)}</Badge>
                        </div>
                        <p className="mt-1 text-xs text-text-muted">7 days · safe maximum {specialist.baseRubyCost.toLocaleString()} rubies per renewal</p>
                      </div>
                      <Switch
                        checked={enabled}
                        onChange={(value) => updateSpecialist(specialist, {
                          enabled: value,
                          minimumDays: Math.max(AUTO_BUYER_MINIMUM_SPECIALIST_DAYS, rule?.minimumDays ?? 0),
                          maximumRubyCostPerPurchase: Math.max(specialist.baseRubyCost, rule?.maximumRubyCostPerPurchase ?? 0),
                        })}
                        ariaLabel={`Maintain ${specialist.name}`}
                      />
                    </div>
                    {enabled ? (
                      <div className="mt-3 grid gap-3 border-t border-border-base pt-3 md:grid-cols-2">
                        <NumberField
                          label="Minimum remaining days"
                          value={rule?.minimumDays ?? AUTO_BUYER_MINIMUM_SPECIALIST_DAYS}
                          minimum={AUTO_BUYER_MINIMUM_SPECIALIST_DAYS}
                          maximum={365}
                          onChange={(minimumDays) => updateSpecialist(specialist, { minimumDays })}
                        />
                        <NumberField
                          label="Max rubies per 7-day renewal"
                          value={rule?.maximumRubyCostPerPurchase ?? specialist.baseRubyCost}
                          minimum={specialist.baseRubyCost}
                          onChange={(maximumRubyCostPerPurchase) => updateSpecialist(specialist, { maximumRubyCostPerPurchase })}
                        />
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </div>
          </Card>
        ) : null}

        {projection && section === 'feast' ? (
          <div className="space-y-3">
            <Card variant="solid" className="p-4">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <h3 className="text-sm font-black text-text-main">Maintain a food production feast</h3>
                  <p className="mt-1 text-xs text-text-muted">The selected feast is started or extended one official duration at a time. Auto Buyer waits for a different active feast to finish.</p>
                </div>
                <Switch
                  checked={draft.feast.enabled}
                  onChange={(enabled) => setDraft((current) => ({ ...current, feast: { ...current.feast, enabled } }))}
                  ariaLabel="Maintain a feast"
                />
              </div>
              {draft.feast.enabled ? (
                <div className="mt-4 grid gap-4 border-t border-border-base pt-4 md:grid-cols-2">
                  <label className="block md:col-span-2">
                    <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Feast</span>
                    <Select
                      value={String(draft.feast.feastId)}
                      onChange={(value) => {
                        const feastId = Number(value);
                        const feast = projection.feasts.find((candidate) => candidate.id === feastId);
                        setDraft((current) => ({
                          ...current,
                          feast: {
                            ...current.feast,
                            feastId,
                            allowRubies: feast?.price.premium ? current.feast.allowRubies : false,
                            maximumRubyCostPerPurchase: feast?.price.premium
                              ? Math.max(current.feast.maximumRubyCostPerPurchase, feast.price.amount)
                              : 0,
                          },
                        }));
                      }}
                      options={projection.feasts.map((feast) => ({
                        value: String(feast.id),
                        label: `${feast.name} · +${feast.productionBoostPercent}% · ${formatFeastPrice(feast)}`,
                      }))}
                      placeholder="Choose an official feast"
                      menuGrowToViewport
                    />
                  </label>
                  <NumberField
                    label="Minimum remaining hours"
                    value={draft.feast.minimumRemainingHours}
                    minimum={1}
                    maximum={24 * 30}
                    onChange={(minimumRemainingHours) => setDraft((current) => ({ ...current, feast: { ...current.feast, minimumRemainingHours } }))}
                  />
                  <label className="block">
                    <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Pay from castle</span>
                    <Select
                      value={String(draft.feast.sourceCastleId || draft.sourceCastleId || '')}
                      onChange={(value) => setDraft((current) => ({ ...current, feast: { ...current.feast, sourceCastleId: Number(value) || 0 } }))}
                      options={castles.map((castle) => ({ value: String(castle.id), label: `${castle.name} · ${castle.x}:${castle.y}` }))}
                      placeholder="Choose the main castle"
                      menuGrowToViewport
                    />
                  </label>
                  {selectedFeast?.price.premium ? (
                    <>
                      <div className="flex items-center justify-between gap-3 rounded-xl border border-warning/30 bg-warning/5 p-3">
                        <div>
                          <div className="text-sm font-bold text-text-main">Allow rubies for this feast</div>
                          <p className="mt-0.5 text-xs text-text-muted">Still preserves the global ruby reserve.</p>
                        </div>
                        <Switch
                          checked={draft.feast.allowRubies}
                          onChange={(allowRubies) => setDraft((current) => ({ ...current, feast: { ...current.feast, allowRubies } }))}
                          ariaLabel="Allow ruby feast"
                        />
                      </div>
                      <NumberField
                        label="Max rubies per feast purchase"
                        value={draft.feast.maximumRubyCostPerPurchase}
                        minimum={selectedFeast.price.amount}
                        onChange={(maximumRubyCostPerPurchase) => setDraft((current) => ({ ...current, feast: { ...current.feast, maximumRubyCostPerPurchase } }))}
                      />
                    </>
                  ) : (
                    <NumberField
                      label={`Keep at least ${selectedFeast?.price.name ?? 'food'}`}
                      value={draft.feast.minimumFoodReserve}
                      minimum={0}
                      onChange={(minimumFoodReserve) => setDraft((current) => ({ ...current, feast: { ...current.feast, minimumFoodReserve } }))}
                    />
                  )}
                </div>
              ) : null}
              <div className="mt-3 flex items-center gap-2 text-xs text-text-muted">
                <Clock3 className="h-3.5 w-3.5" /> Current feast: {formatRemaining(state?.market.feast?.expiresAt)}
              </div>
            </Card>

            <Card variant="solid" className="border-border-base p-4">
              <div className="flex items-start gap-3">
                <Sparkles className="mt-0.5 h-5 w-5 shrink-0 text-text-muted" />
                <div>
                  <h3 className="text-sm font-black text-text-main">Timed ruby offers are staged for a later release</h3>
                  <p className="mt-1 text-xs text-text-muted">{projection.timedOffers.reason ?? 'Unattended timed-offer purchases are disabled.'} The future flow will require an item match, a user ceiling, and the server-confirmed quote before purchase.</p>
                </div>
              </div>
            </Card>
          </div>
        ) : null}
      </div>
    </SettingsModal>
  );
};

function NumberField({
  label,
  value,
  minimum,
  maximum = Number.MAX_SAFE_INTEGER,
  onChange,
}: {
  label: string;
  value: number;
  minimum: number;
  maximum?: number;
  onChange: (value: number) => void;
}) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">{label}</span>
      <Input
        type="number"
        min={minimum}
        max={maximum}
        value={value}
        onChange={(event) => onChange(clampAutoBuyerInteger(event.target.value, minimum, maximum, minimum))}
      />
    </label>
  );
}

function SectionButton({
  active,
  onClick,
  icon,
  children,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <Button variant={active ? 'primary' : 'secondary'} onClick={onClick} leftIcon={icon}>
      <span className="flex items-center gap-2">{children}</span>
    </Button>
  );
}

function formatPrice(product: AutoBuyerPackageV1): string {
  return `${product.price.amount.toLocaleString()} ${product.price.name}`;
}

function autoBuyerCurrencyKey(product: AutoBuyerPackageV1): string {
  const { price } = product;
  if (price.scope === 'currency' && price.currencyId !== undefined) {
    return `currency:${price.currencyId}`;
  }
  if (price.resourceId !== undefined) {
    return `${price.scope}:${price.resourceId}`;
  }
  return `${price.scope}:${price.jsonKey ?? price.field}`;
}

function formatFeastPrice(feast: AutoBuyerFeastV1): string {
  return `${feast.price.amount.toLocaleString()} ${feast.price.name}`;
}

function formatRemaining(expiresAt: string | undefined): string {
  if (!expiresAt) return 'Inactive';
  const remainingMs = Date.parse(expiresAt) - Date.now();
  if (!Number.isFinite(remainingMs) || remainingMs <= 0) return 'Inactive';
  const hours = Math.ceil(remainingMs / 3_600_000);
  const days = Math.floor(hours / 24);
  return days > 0 ? `${days}d ${hours % 24}h left` : `${hours}h left`;
}

export default AutoBuyerSettingsModal;
