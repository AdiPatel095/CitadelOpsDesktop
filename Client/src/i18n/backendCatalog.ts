import type { Catalog } from './formatMessage';
import { normalizeLocale } from './locales';
const loaders = import.meta.glob<{ default: Catalog }>(['./backend/*.json','!./backend/provenance.json']);
const loaded = new Map<string, Promise<Catalog>>();
/** Only the selected authored pack is loaded; English remains an explicit fallback. */
export function loadBackendCatalog(language: string): Promise<Catalog> {
    const locale = normalizeLocale(language);
    if (!locale || locale === 'en') return Promise.resolve({});
    const load = loaders[`./backend/${locale}.json`];
    if (!load) return Promise.resolve({});
    if (!loaded.has(locale)) loaded.set(locale, load().then(module => module.default).catch(() => ({})));
    return loaded.get(locale)!;
}
