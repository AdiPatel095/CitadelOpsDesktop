import type { LocalizedMessage } from './formatMessage.ts';

const record = (value: unknown): Record<string, unknown> | undefined =>
    value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined;

/** Validate the additive server contract before letting it reach ICU formatting. */
export function parseMessageDescriptor(value: unknown): LocalizedMessage | undefined {
    const input = record(value);
    if (!input || typeof input.key !== 'string' || !input.key.trim() || typeof input.fallback !== 'string' || !input.fallback.trim()) return undefined;
    if (input.key.length > 512 || input.fallback.length > 16_384) return undefined;
    const params: LocalizedMessage['params'] = {};
    if (input.params !== undefined) {
        const values = record(input.params);
        if (!values || Object.keys(values).length > 64) return undefined;
        for (const [key, item] of Object.entries(values)) {
            if (key === '__proto__' || key === 'prototype' || key === 'constructor') return undefined;
            if (typeof item !== 'string' && typeof item !== 'number' && typeof item !== 'boolean') return undefined;
            if (typeof item === 'number' && !Number.isFinite(item)) return undefined;
            params[key] = item;
        }
    }
    let context: LocalizedMessage['context'];
    if (input.context !== undefined) {
        if (!Array.isArray(input.context) || input.context.length > 4) return undefined;
        context = [];
        for (const item of input.context) {
            const leaf = record(item);
            if (!leaf || Object.keys(leaf).some(key => !['key', 'fallback', 'params', 'fallbackText', 'officialKey', 'officialParams', 'gameParams'].includes(key))) return undefined;
            const parsed = parseMessageDescriptor(leaf);
            if (!parsed) return undefined;
            context.push(parsed);
        }
    }
    const extra: Pick<LocalizedMessage, 'officialKey' | 'officialParams' | 'gameParams' | 'fallbackText'> = {};
    if (input.fallbackText !== undefined) {
        if (typeof input.fallbackText !== 'string' || input.fallbackText.length > 16_384) return undefined;
        extra.fallbackText = input.fallbackText;
    }
    if (input.officialKey !== undefined) {
        if (typeof input.officialKey !== 'string' || !input.officialKey.trim() || input.officialKey.length > 512) return undefined;
        extra.officialKey = input.officialKey;
    }
    if (input.officialParams !== undefined) {
        const parsed = parseMessageDescriptor({ key: 'validation', fallback: 'validation', params: input.officialParams });
        if (!parsed || !extra.officialKey) return undefined;
        extra.officialParams = parsed.params;
    }
    if (input.gameParams !== undefined) {
        const values = record(input.gameParams);
        if (!values || Object.keys(values).length > 64) return undefined;
        extra.gameParams = {};
        for (const [name, value] of Object.entries(values)) {
            if (name === '__proto__' || name === 'constructor' || name === 'prototype') return undefined;
            const noun = record(value);
            if (!noun || Object.keys(noun).some(key => !['key', 'fallback', 'params'].includes(key))) return undefined;
            const parsed = parseMessageDescriptor(noun);
            if (!parsed) return undefined;
            extra.gameParams[name] = { key: parsed.key, fallback: parsed.fallback, ...(parsed.params ? { params: parsed.params } : {}) };
        }
    }
    let listParams:LocalizedMessage['listParams'];
    if(input.listParams!==undefined){
        const lists=record(input.listParams);
        if(input.officialKey!==undefined || !lists || typeof input.fallbackText!=='string' || Object.keys(lists).length<1 || Object.keys(lists).length>4)return undefined;
        try {if(new TextEncoder().encode(JSON.stringify(lists).replace(/[<>&\u2028\u2029]/g,char=>'\\u'+char.charCodeAt(0).toString(16).padStart(4,'0'))).byteLength>65_536)return undefined;} catch{return undefined;}
        listParams={};let count=0;
        for(const [name,items] of Object.entries(lists)){
            if(['__proto__','prototype','constructor'].includes(name) || Object.hasOwn(params,name) || Object.hasOwn(extra.gameParams??{},name))return undefined;
            if(!Array.isArray(items) || items.length<1 || items.length>32 || (count+=items.length)>64)return undefined;
            const leaves=[];
            for(const item of items){
                const leaf=record(item);
                if(!leaf || Object.keys(leaf).some(key=>!['key','fallback','params','fallbackText','officialKey','officialParams','gameParams'].includes(key)))return undefined;
                const parsed=parseMessageDescriptor(leaf);if(!parsed)return undefined;
                leaves.push(parsed);
            }
            listParams[name]=leaves;
        }
    }
    return { key: input.key, fallback: input.fallback, ...(input.params !== undefined ? { params } : {}), ...(context ? { context } : {}), ...(listParams?{listParams}:{}), ...extra };
}

export function responseMessageDescriptor(payload: unknown): LocalizedMessage | undefined {
    const body = record(payload);
    if (!body) return undefined;
    return parseMessageDescriptor(record(body.error)?.messageDescriptor) ?? parseMessageDescriptor(body.messageDescriptor);
}
