import { localeStorageKey, normalizeLocale } from './locales.ts';
import type { Locale } from './locales.ts';

export type ViewerLocalePreference = Locale | 'auto';
export const viewerLocaleEvent = 'citadelops:viewer-locale';
const sharedKey = Symbol.for('citadelops.viewer-locale.session');
const shared = globalThis as unknown as Record<symbol, { preference?: ViewerLocalePreference }>;
const session = shared[sharedKey] ??= {};

export function readViewerLocalePreference(): ViewerLocalePreference {
    if (session.preference) return session.preference;
    try { return normalizeLocale(globalThis.localStorage?.getItem(localeStorageKey)) ?? 'auto'; }
    catch { return 'auto'; }
}

export function resolveDeviceLocale(): Locale {
    if (typeof navigator === 'undefined') return 'en';
    for (const language of [...(navigator.languages ?? []), navigator.language]) {
        const locale = normalizeLocale(language);
        if (locale) return locale;
    }
    return 'en';
}

/** Primitive snapshots stay stable for useSyncExternalStore. */
export function readViewerLocale(): Locale {
    const preference = readViewerLocalePreference();
    return preference === 'auto' ? resolveDeviceLocale() : preference;
}

export function setViewerLocale(value: string): void {
    const preference = value === 'auto' ? 'auto' : normalizeLocale(value);
    if (!preference) return;
    session.preference = preference;
    try {
        if (preference === 'auto') globalThis.localStorage?.removeItem(localeStorageKey);
        else globalThis.localStorage?.setItem(localeStorageKey, preference);
    } catch { /* In-memory preference remains usable. */ }
    if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent(viewerLocaleEvent, { detail: preference }));
}

export function subscribeViewerLocale(notify: () => void): () => void {
    if (typeof window === 'undefined') return () => {};
    const changed = (event: Event) => {
        const detail = (event as CustomEvent<unknown>).detail;
        const next = detail === 'auto' ? 'auto' : normalizeLocale(detail);
        if (!next) return;
        session.preference = next;
        notify();
    };
    const stored = (event: StorageEvent) => {
        if (event.key !== localeStorageKey && event.key !== null) return;
        // Ignore sessionStorage and unrelated storage areas. Synthetic events may omit the area.
        try { if (event.storageArea && event.storageArea !== globalThis.localStorage) return; } catch { return; }
        session.preference = normalizeLocale(event.newValue) ?? 'auto';
        notify();
    };
    const languageChanged = () => { if (readViewerLocalePreference() === 'auto') notify(); };
    window.addEventListener(viewerLocaleEvent, changed);
    window.addEventListener('storage', stored);
    window.addEventListener('languagechange', languageChanged);
    return () => {
        window.removeEventListener(viewerLocaleEvent, changed);
        window.removeEventListener('storage', stored);
        window.removeEventListener('languagechange', languageChanged);
    };
}
