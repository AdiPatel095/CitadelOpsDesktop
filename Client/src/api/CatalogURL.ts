/**
 * URL of one game-data collection (CIT-35).
 *
 * The worker caches a collection by the game-data content digest: a URL that
 * names the current digest is immutable (cached for a year) and one without it
 * is revalidated with an ETag. The locale is part of the URL as well, so
 * languages never share a cache entry. The digest comes from the catalog
 * manifest (`metadata.digestSha256`); until the manifest is known the URL simply
 * omits it.
 *
 * This file is shared byte-for-byte between the desktop client and the hosted
 * command center; keep it free of imports.
 */
export function catalogPath(name: string, locale?: string, digest?: string): string {
	const query: string[] = [];
	const cleanLocale = locale?.trim();
	if (cleanLocale) query.push(`locale=${encodeURIComponent(cleanLocale)}`);
	const cleanDigest = digest?.trim();
	if (cleanDigest) query.push(`digest=${encodeURIComponent(cleanDigest)}`);
	return `/api/v2/game-data/${encodeURIComponent(name)}${query.length > 0 ? `?${query.join('&')}` : ''}`;
}

/** The digest a manifest names, or '' when it carries none (an older worker). */
export function manifestDigest(manifest: unknown): string {
	if (typeof manifest !== 'object' || manifest === null) return '';
	const metadata = (manifest as { metadata?: unknown }).metadata;
	if (typeof metadata !== 'object' || metadata === null) return '';
	const digest = (metadata as { digestSha256?: unknown }).digestSha256;
	return typeof digest === 'string' ? digest.trim() : '';
}
