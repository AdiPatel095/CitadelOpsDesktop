import type { Locale } from './locales';
import type { Catalog } from './formatMessage';
const loaders = import.meta.glob<{default: Catalog}>('./catalogs/*.json');
/** Model-authored catalogs; human linguistic review is still pending. */
export async function loadMessageCatalog(locale: Locale): Promise<Catalog> {
  if (locale === 'en') return {};
  const loader = loaders[`./catalogs/${locale}.json`];
  if (!loader) return {};
  return (await loader()).default;
}
