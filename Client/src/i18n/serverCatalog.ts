import type { Locale } from './locales';
import type { Catalog } from './formatMessage';
const loaders = import.meta.glob<{default: Catalog}>(['./server/*.json','!./server/coverage.json','!./server/provenance.json']);
/** Partial authored server packs; missing keys use descriptor fallback with truthful provenance. */
export async function loadServerCatalog(locale: Locale): Promise<Catalog> {
  if (locale === 'en') return {};
  const loader = loaders[`./server/${locale}.json`];
  return loader ? (await loader()).default : {};
}
