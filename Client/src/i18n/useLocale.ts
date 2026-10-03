import { createContext, useContext } from 'react';
import type { Locale } from './locales';
import type { ViewerLocalePreference } from './viewerLocaleStore';
import { describeMessage } from './messages';
import { formatMessage } from './formatMessage';
import type { Catalog, OfficialCatalog } from './formatMessage';
import type { MessageKey, MessageParameters } from './messages';

export function createValue(locale: Locale, setLocale: (locale: ViewerLocalePreference) => void, catalog: Catalog = {}, game?: OfficialCatalog, runtimeStatus = 'Disconnected', preference: ViewerLocalePreference = 'auto') {
  const message = (key: MessageKey, parameters?: MessageParameters) => formatMessage(describeMessage(key,parameters),locale,catalog,game);
  return {
    message, catalog, runtimeStatus,
    locale, preference, setLocale, direction: locale === 'ar' ? 'rtl' as const : 'ltr' as const,
    messageLocale: Object.keys(catalog).length ? locale : 'en',
    t: (key: MessageKey, parameters?: MessageParameters) => message(key,parameters).text,
    number: (value: number, options?: Intl.NumberFormatOptions) => new Intl.NumberFormat(locale,options).format(value),
    date: (value: Date | number, options?: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat(locale,options).format(value),
    plural: (value: number, options?: Intl.PluralRulesOptions) => new Intl.PluralRules(locale,options).select(value),
  };
}
export const LocaleContext = createContext(createValue('en', () => {}));

export function useLocale() { return useContext(LocaleContext); }
