/** A mixed sentence must not falsely claim a single inherited language. */
export function messageLanguageAttributes(result:{resolvedLocale:string}) {
    const mixed=result.resolvedLocale==='mixed';
    return {
        lang:mixed?'':result.resolvedLocale,
        dir:mixed?'auto':result.resolvedLocale==='ar'?'rtl':'ltr',
        'data-message-locale':result.resolvedLocale,
    };
}
