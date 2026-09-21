import { IntlMessageFormat } from 'intl-messageformat';
import { parse, TYPE } from '@formatjs/icu-messageformat-parser';
import type { MessageFormatElement } from '@formatjs/icu-messageformat-parser';
export type MessageValue = string | number | boolean;
export type OfficialMessage = {key: string; fallback: string; params?: Record<string, MessageValue>};
export type LocalizedMessage = {key: string; fallback: string; params?: Record<string, MessageValue>; officialKey?: string; officialParams?: Record<string, MessageValue>; gameParams?: Record<string, OfficialMessage>};
export type OfficialCatalog = {values: Readonly<Record<string,string>>; resolvedLocale: string; fallbackKeys?: readonly string[]};
function officialText(template: string, params: Record<string, MessageValue> = {}): string {
  return template.replace(/\{([A-Za-z0-9_]+)\}/g, (token, key: string) => Object.hasOwn(params,key) ? String(params[key]) : token);
}
export type Catalog = Readonly<Record<string,string>>;
/** Plain text only. React renders the result as text, never HTML. */
export function formatMessage(message: LocalizedMessage, locale: string, catalog: Catalog, game?: OfficialCatalog): {text: string; translated: boolean; resolvedLocale: string} {
  if (message.officialKey && game && Object.hasOwn(game.values,message.officialKey)) {
    const translated = game.resolvedLocale === locale && !(game.fallbackKeys ?? []).includes(message.officialKey);
    return {text:officialText(game.values[message.officialKey],message.officialParams),translated,resolvedLocale:translated ? locale : 'en'};
  }
  const hasGameFallback = Object.values(message.gameParams ?? {}).some(noun => !game || game.resolvedLocale !== locale || !Object.hasOwn(game.values,noun.key) || (game.fallbackKeys ?? []).includes(noun.key));
  const gameParams = Object.fromEntries(Object.entries(message.gameParams ?? {}).map(([name,noun]) => [name,
    officialText(game?.values[noun.key] ?? noun.fallback,noun.params),
  ]));
  const translated = Object.hasOwn(catalog,message.key) && typeof catalog[message.key] === 'string';
  const params = Object.fromEntries(Object.entries({...message.params,...gameParams}).map(([key,value]) => [key, typeof value === 'boolean' ? String(value) : value]));
  const render = (template: string, language: string) => {
    const result = new IntlMessageFormat(template,language,undefined,{ignoreTag:true}).format(params);
    return Array.isArray(result) ? result.join('') : String(result);
  };
  if (translated) {
    try { return {text: render(catalog[message.key],locale),translated:!hasGameFallback,resolvedLocale:hasGameFallback ? 'mixed' : locale}; }
    catch { /* Invalid translations never break the surrounding user interface. */ }
  }
  try { return {text:render(message.fallback,'en'),translated:false,resolvedLocale:'en'}; }
  catch { return {text:message.fallback,translated:false,resolvedLocale:'en'}; }

}
export function messageArguments(template: string): string[] {
  const found = new Set<string>();
  function visit(elements: MessageFormatElement[]) {
    for (const element of elements) {
      if (element.type !== TYPE.literal && element.type !== TYPE.pound) found.add(element.value);
      if (element.type === TYPE.select || element.type === TYPE.plural) for (const option of Object.values(element.options)) visit(option.value);
      if (element.type === TYPE.tag) visit(element.children);
    }
  }
  visit(parse(template,{ignoreTag:true}));
  return [...found].sort();
}
export function validateMessageCatalog(source: Catalog, catalog: Catalog): string[] {
  const errors: string[] = [];
  for (const key of Object.keys(source)) {
    if (!Object.hasOwn(catalog,key) || !catalog[key].trim()) { errors.push(`Missing message: ${key}`); continue; }
    try { if (JSON.stringify(messageArguments(source[key])) !== JSON.stringify(messageArguments(catalog[key]))) errors.push(`Argument mismatch: ${key}`); }
    catch { errors.push(`Invalid ICU message: ${key}`); }
  }
  for (const key of Object.keys(catalog)) if (!Object.hasOwn(source,key)) errors.push(`Unknown message: ${key}`);
  return errors;
}
