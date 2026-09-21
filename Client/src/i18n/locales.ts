/** Viewer locales are independent of account login and game-server language. */
export const localeCodes = ['en', 'de', 'fr', 'pl', 'ru', 'it', 'nl', 'pt', 'es', 'ar', 'da', 'no', 'fi', 'sv', 'ja', 'ko', 'el', 'tr', 'zh-CN', 'zh-TW', 'cs', 'ro', 'sk', 'hu', 'bg', 'lt'] as const;
export type Locale = typeof localeCodes[number];
export type LocaleDefinition = { code: Locale; gameCode: string; nativeName: string; name: string; direction: 'ltr' | 'rtl' };
const names = ['English','Deutsch','Français','Polski','Русский','Italiano','Nederlands','Português','Español','العربية','Dansk','Norsk','Suomi','Svenska','日本語','한국어','Ελληνικά','Türkçe','简体中文','繁體中文','Čeština','Română','Slovenčina','Magyar','Български','Lietuvių'];
export const locales: readonly LocaleDefinition[] = localeCodes.map((code, index) => ({code, gameCode: code.startsWith('zh-') ? code.replace('-', '_') : code, nativeName: names[index], name: names[index], direction: code === 'ar' ? 'rtl' : 'ltr'}));
export function normalizeLocale(value: unknown): Locale | undefined {
  if (typeof value !== 'string') return undefined;
  const tag = value.trim().replaceAll('_', '-').toLowerCase();
  const exact = localeCodes.find(code => code.toLowerCase() === tag);
  if (exact) return exact;
  if (/^zh-(hant|tw|hk)(-|$)/.test(tag)) return 'zh-TW';
  if (/^zh(-|$)/.test(tag)) return 'zh-CN';
  if (/^(nb|nn)(-|$)/.test(tag)) return 'no';
  return localeCodes.find(code => code === tag.split('-')[0]);
}
export const localeStorageKey = 'citadelops.viewer-locale';
