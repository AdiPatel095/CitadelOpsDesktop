import { useEffect, useState } from 'react';
import { useLocale } from '../i18n/LocaleContext';
import english from './guideLocales/en.json';

export type GuidePack = typeof english;
export const englishGuidePack: GuidePack = english;
const loaders = import.meta.glob<{ default: GuidePack }>('./guideLocales/*.json');
const translatedSource = { ...english, ui: { ...english.ui }, panels: { ...english.panels } };
delete (translatedSource as Partial<GuidePack>).autoFortress;
delete (translatedSource as Partial<GuidePack>).autoInvasion;
delete (translatedSource as Partial<GuidePack>).autoNomad;
delete (translatedSource as Partial<GuidePack>).autoAdvisor;
delete (translatedSource as Partial<GuidePack>).autoKhan;
delete (translatedSource as Partial<GuidePack>).autoBeri;
delete (translatedSource.ui as Record<string, string>).fortressGuideTitle;
delete (translatedSource.ui as Record<string, string>).fortressGuideIntro;
delete (translatedSource.ui as Record<string, string>).fortressPreviewTitle;
for (const key of ['beriGuideTitle', 'beriGuideIntro', 'beriPreviewTitle']) delete (translatedSource.ui as Record<string, string>)[key];
for (const key of ['khanGuideTitle', 'khanGuideIntro', 'khanPreviewTitle']) delete (translatedSource.ui as Record<string, string>)[key];
for (const key of ['advisorGuideTitle', 'advisorGuideIntro', 'advisorPreviewTitle']) delete (translatedSource.ui as Record<string, string>)[key];
for (const key of ['nomadGuideTitle', 'nomadGuideIntro', 'nomadPreviewTitle']) delete (translatedSource.ui as Record<string, string>)[key];
for (const key of ['invasionGuideTitle', 'invasionGuideIntro', 'invasionPreviewTitle']) delete (translatedSource.ui as Record<string, string>)[key];
for (const key of ['fortressKingdoms', 'fortressSupply', 'fortressAttack', 'invasionSetup', 'invasionDifficulty', 'invasionLimits', 'invasionFortify', 'nomadSetup', 'nomadDifficulty', 'nomadLimits', 'nomadCooldowns', 'nomadTrial', 'advisorSetup', 'advisorDifficulty', 'advisorSizing', 'advisorResources', 'advisorActivation', 'advisorOverview', 'khanSetup', 'khanRage', 'khanLimits', 'khanCooldowns', 'khanProtection', 'khanSave', 'beriSetup', 'beriTransfers', 'beriReadiness', 'beriTools', 'beriBuilder', 'beriSave']) delete (translatedSource.panels as Record<string, unknown>)[key];

function complete(candidate: unknown, source: unknown): boolean {
  if (typeof source === 'string') return typeof candidate === 'string' && candidate.trim().length > 0;
  if (!source || typeof source !== 'object' || Array.isArray(source)) return false;
  if (!candidate || typeof candidate !== 'object' || Array.isArray(candidate)) return false;
  return Object.entries(source).every(([key, value]) => complete((candidate as Record<string, unknown>)[key], value));
}

/** Keep the source language explicit while a selected pack loads. */
export function useGuideLocale() {
  const { locale } = useLocale();
  const [loaded, setLoaded] = useState<{ locale: string; pack: GuidePack }>({ locale: 'en', pack: english });
  useEffect(() => {
    if (locale === 'en') return;
    let active = true;
    const loader = loaders[`./guideLocales/${locale}.json`];
    if (loader) void loader().then((module) => { if (active && complete(module.default, translatedSource)) setLoaded({ locale, pack: module.default }); }).catch(() => { /* Source-language fallback remains explicit. */ });
    return () => { active = false; };
  }, [locale]);
  return locale === 'en' ? { locale: 'en', pack: english } : loaded.locale === locale ? loaded : { locale: 'en', pack: english };
}
