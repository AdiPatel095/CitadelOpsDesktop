import { localeStorageKey, normalizeLocale } from './locales.ts';
import type { Locale } from './locales.ts';

export const viewerLocaleEvent = 'citadelops:viewer-locale';
const sharedKey = Symbol.for('citadelops.viewer-locale.session');
const shared = globalThis as unknown as Record<symbol, { locale?: Locale }>;
const session = shared[sharedKey] ??= {};

/** An unavailable storage backend must not discard a same-tab selection. */
export function readViewerLocale(): Locale {
    if (session.locale) return session.locale;
    try { return normalizeLocale(globalThis.localStorage?.getItem(localeStorageKey)) ?? 'en'; }
    catch { return 'en'; }
}

export function setViewerLocale(value: string): void {
    const locale = normalizeLocale(value);
    if (!locale) return;
    session.locale = locale;
    try { globalThis.localStorage?.setItem(localeStorageKey, locale); } catch { /* In-memory selection remains usable. */ }
    if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent(viewerLocaleEvent, { detail: locale }));
}

export function subscribeViewerLocale(notify: () => void): () => void {
    if (typeof window === 'undefined') return () => {};
    const changed = (event: Event) => {
        const next = normalizeLocale((event as CustomEvent<unknown>).detail);
        if (!next) return;
        session.locale = next;
        notify();
    };
    const stored = (event: StorageEvent) => {
        if (event.key !== localeStorageKey && event.key !== null) return;
        session.locale = normalizeLocale(event.newValue) ?? 'en';
        notify();
    };
    window.addEventListener(viewerLocaleEvent, changed);
    window.addEventListener('storage', stored);
    return () => { window.removeEventListener(viewerLocaleEvent, changed); window.removeEventListener('storage', stored); };
}
