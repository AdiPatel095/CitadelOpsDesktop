import { SectionCard, Select } from '../components/ui';
import { useLocale } from './LocaleContext';
import { locales, normalizeLocale } from './locales';

/** Viewer-only preference: no account settings or game-language mutations. */
export function LanguageSelector() {
  const {preference,setLocale,t,messageLocale} = useLocale();
  return <SectionCard title={<span lang={messageLocale}>{t('locale.select')}</span>}
    description={<span lang={messageLocale}>{t('locale.help')}</span>} contentClassName="p-6 space-y-3">
    <Select ariaLabel={t('locale.select')} value={preference}
      options={[
        {value:'auto',label:<span lang={messageLocale}>{t('locale.automatic')}</span>},
        ...locales.map(item => ({value:item.code,label:<span lang={item.code} dir={item.direction}>{item.nativeName}</span>})),
      ]}
      onChange={value => { const next = value === 'auto' ? 'auto' : normalizeLocale(value); if (next) setLocale(next); }} />
    <p lang={messageLocale} className="text-xs text-text-muted">{t('locale.coverage')}</p>
  </SectionCard>;
}
