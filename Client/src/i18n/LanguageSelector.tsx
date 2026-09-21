import { useEffect, useState } from 'react';
import { CitadelAPI } from '../api/CitadelClient';
import { useLocale } from './LocaleContext';
import { locales, normalizeLocale } from './locales';
export function LanguageSelector() {
  const {locale,setLocale,t} = useLocale();
  const [available,setAvailable] = useState(locales);
  useEffect(() => {
    let active = true;
    void CitadelAPI.getLocales().then(manifest => {
      if (!active || manifest.schemaVersion !== 1 || !Array.isArray(manifest.locales)) return;
      const supported = manifest.locales.flatMap(item => {
        const code = normalizeLocale(item.code);
        return code && typeof item.nativeName === 'string' ? [{...item,code}] : [];
      });
      if (supported.length) setAvailable(supported);
    }).catch(() => { /* Bundled official locale list supports offline selection. */ });
    return () => { active = false; };
  },[]);
  return <label className="viewer-language" title={t('locale.coverage')}>
    <span lang="en">{t('locale.select')}</span>
    <select aria-label={t('locale.select')} value={locale} onChange={event => { const next = normalizeLocale(event.target.value); if (next) setLocale(next); }}>
      {available.map(item => <option key={item.code} value={item.code} lang={item.code}>{item.nativeName}</option>)}
    </select>
  </label>;
}
