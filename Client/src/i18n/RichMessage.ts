import { createElement, Fragment } from 'react';
import type { ReactNode } from 'react';
import { isolateMessageArguments, messageSelectOptions } from './formatMessage.ts';
import { IntlMessageFormat } from 'intl-messageformat';
import { parse, TYPE } from '@formatjs/icu-messageformat-parser';
import type { MessageFormatElement } from '@formatjs/icu-messageformat-parser';
import type { Catalog, LocalizedMessage } from './formatMessage.ts';

export type RichMessageContract = { arguments: readonly string[]; tags: readonly string[] };
export type RichMessageTags = Readonly<Record<string, (children: ReactNode[]) => ReactNode>>;

/** Native ICU parsing: no HTML parsing, attributes, or user-defined element names. */
export function richMessageContract(template: string): RichMessageContract {
    const args = new Set<string>();
    const tags = new Set<string>();
    function visit(nodes: MessageFormatElement[]) {
        for (const node of nodes) {
            if (node.type === TYPE.tag) { tags.add(node.value); visit(node.children); }
            else if (node.type !== TYPE.literal && node.type !== TYPE.pound) args.add(node.value);
            if (node.type === TYPE.select || node.type === TYPE.plural) for (const option of Object.values(node.options)) visit(option.value);
        }
    }
    visit(parse(template, { ignoreTag: false }));
    return { arguments: [...args].sort(), tags: [...tags].sort() };
}

const matches = (left: readonly string[], right: readonly string[]) => JSON.stringify([...left].sort()) === JSON.stringify([...right].sort());

export function validateRichMessageCatalog(source: Catalog, catalog: Catalog, contracts: Readonly<Record<string, RichMessageContract>>): string[] {
    const errors: string[] = [];
    for (const [key, contract] of Object.entries(contracts)) {
        try {
            const baseline = richMessageContract(source[key]);
            const translated = richMessageContract(catalog[key]);
            if (!matches(messageSelectOptions(source[key],false),messageSelectOptions(catalog[key],false))) errors.push(`Select option mismatch: ${key}`);
            if (!matches(baseline.arguments, contract.arguments) || !matches(baseline.tags, contract.tags)) errors.push(`Source contract mismatch: ${key}`);
            if (!matches(translated.arguments, contract.arguments) || !matches(translated.tags, contract.tags)) errors.push(`Translation contract mismatch: ${key}`);
        } catch { errors.push(`Invalid rich message: ${key}`); }
    }
    return errors;
}

export function renderRichMessage(message: LocalizedMessage, locale: string, catalog: Catalog, tags: RichMessageTags): { content: ReactNode; translated: boolean } {
    const render = (template: string, language: string): ReactNode => {
        const source = richMessageContract(message.fallback);
        const candidate = richMessageContract(template);
        if (!matches(source.tags, Object.keys(tags)) || !matches(source.arguments, Object.keys(message.params ?? {})) || !matches(source.tags, candidate.tags) || !matches(source.arguments, candidate.arguments)) throw new Error('Rich message contract mismatch');
        if (!matches(messageSelectOptions(message.fallback,false),messageSelectOptions(template,false))) throw new Error('Rich select options mismatch');
        const params = Object.fromEntries(Object.entries(message.params ?? {}).map(([key, value]) => [key, typeof value === 'boolean' ? String(value) : value]));
        // Each callback is supplied by application code and retains its own href,
        // rel, event handlers and accessibility behavior. Text stays React text.
        const parsed = parse(template, { ignoreTag: false });
        const value = new IntlMessageFormat(/^ar(-|$)/.test(language) ? isolateMessageArguments(parsed) : parsed, language, undefined, { ignoreTag: false }).format<ReactNode>({ ...params, ...tags });
        return createElement(Fragment, null, ...(Array.isArray(value) ? value : [value]));
    };
    if (Object.hasOwn(catalog, message.key)) {
        try { return { content: render(catalog[message.key], locale), translated: true }; }
        catch { /* Reject malformed translations and retry the source contract. */ }
    }
    try { return { content: render(message.fallback, 'en'), translated: false }; }
    catch { return { content: message.fallback, translated: false }; }
}

export function RichMessage(props: { message: LocalizedMessage; locale: string; catalog: Catalog; tags: RichMessageTags }): ReactNode {
    return renderRichMessage(props.message, props.locale, props.catalog, props.tags).content;
}
