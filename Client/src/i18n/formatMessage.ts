import { IntlMessageFormat } from 'intl-messageformat';
import { parse, TYPE } from '@formatjs/icu-messageformat-parser';
import type { MessageFormatElement } from '@formatjs/icu-messageformat-parser';
export type MessageValue = string | number | boolean;
export type OfficialMessage = {key: string; fallback: string; params?: Record<string, MessageValue>};
export type MessageLeaf = {key: string; fallback: string; fallbackText?: string; params?: Record<string, MessageValue>; officialKey?: string; officialParams?: Record<string, MessageValue>; gameParams?: Record<string, OfficialMessage>};
export type LocalizedMessage = MessageLeaf & {context?: MessageLeaf[]};
export type OfficialCatalog = {values: Readonly<Record<string,string>>; resolvedLocale: string; fallbackKeys?: readonly string[]};
function officialText(template: string, params: Record<string, MessageValue> = {}, locale = 'en'): string {
  return template.replace(/\{([A-Za-z0-9_]+)\}/g, (token, key: string) => Object.hasOwn(params,key) ? (/^ar(-|$)/.test(locale) ? `\u2068${String(params[key])}\u2069` : String(params[key])) : token);
}
function officialParametersComplete(template: string, params: Record<string, MessageValue> = {}): boolean {
  return [...template.matchAll(/\{([A-Za-z0-9_]+)\}/g)].every(match => Object.hasOwn(params,match[1]));
}
/** Isolate rendered arguments, never the selector input or stored user values. */
function isolateArguments(elements: MessageFormatElement[]): MessageFormatElement[] {
  return elements.flatMap(element=>{
    if (element.type === TYPE.select || element.type === TYPE.plural) return [{...element,options:Object.fromEntries(Object.entries(element.options).map(([key,option])=>[key,{...option,value:isolateArguments(option.value)}]))}];
    if (element.type === TYPE.literal || element.type === TYPE.tag) return [element];
    return [{type:TYPE.literal,value:'\u2068'},element,{type:TYPE.literal,value:'\u2069'}];
  });
}
export type Catalog = Readonly<Record<string,string>>;
/** Plain text only. React renders the result as text, never HTML. */
function formatSingleMessage(message: LocalizedMessage, locale: string, catalog: Catalog, game?: OfficialCatalog): {text: string; translated: boolean; resolvedLocale: string; usedFallbackText?:boolean} {
  const officialIncomplete = !!(message.officialKey && game && Object.hasOwn(game.values,message.officialKey) && !officialParametersComplete(game.values[message.officialKey],message.officialParams));
  if (message.officialKey && game && Object.hasOwn(game.values,message.officialKey) && officialParametersComplete(game.values[message.officialKey],message.officialParams)) {
    const translated = game.resolvedLocale === locale && !(game.fallbackKeys ?? []).includes(message.officialKey);
    return {text:officialText(game.values[message.officialKey],message.officialParams,locale),translated,resolvedLocale:translated ? locale : 'en'};
  }
  const nouns = Object.entries(message.gameParams ?? {}).map(([name,noun]) => {
    const template = game?.values[noun.key];
    const usable = typeof template === 'string' && officialParametersComplete(template,noun.params);
    const translated = usable && game?.resolvedLocale === locale && !(game.fallbackKeys ?? []).includes(noun.key);
    return {name,text:officialText(usable ? template : noun.fallback,noun.params,locale),translated};
  });
  const hasGameFallback = nouns.some(noun => !noun.translated);
  const hasTranslatedNoun = locale !== 'en' && nouns.some(noun => noun.translated);
  const gameParams = Object.fromEntries(nouns.map(noun => [noun.name,noun.text]));
  const translated = Object.hasOwn(catalog,message.key) && typeof catalog[message.key] === 'string';
  const params = Object.fromEntries(Object.entries({...message.params,...gameParams}).map(([key,value]) => [key, typeof value === 'boolean' ? String(value) : value]));
  const render = (template: string, language: string) => {
    const source = /^ar(-|$)/.test(language) ? isolateArguments(parse(template,{ignoreTag:true})) : template;
    const result = new IntlMessageFormat(source,language,undefined,{ignoreTag:true}).format(params);
    return Array.isArray(result) ? result.join('') : String(result);
  };
  if (translated && !officialIncomplete) {
    try { return {text: render(catalog[message.key],locale),translated:!hasGameFallback,resolvedLocale:hasGameFallback ? 'mixed' : locale}; }
    catch { /* Invalid translations never break the surrounding user interface. */ }
  }
  if (typeof message.fallbackText === 'string') return {text:message.fallbackText,translated:false,resolvedLocale:'en',usedFallbackText:true};
  try { return {text:render(message.fallback,'en'),translated:false,resolvedLocale:hasTranslatedNoun ? 'mixed' : 'en'}; }
  catch { return {text:message.fallbackText ?? message.fallback,translated:false,resolvedLocale:'en'}; }

}
/** Context entries are bounded explicit labels, not recursively nested messages. */
export function formatMessage(message: LocalizedMessage, locale: string, catalog: Catalog, game?: OfficialCatalog): {text: string; translated: boolean; resolvedLocale: string} {
  const result = formatSingleMessage(message,locale,catalog,game);
  if (result.usedFallbackText) return {text:result.text,translated:false,resolvedLocale:result.resolvedLocale};
  const context = (message.context ?? []).slice(0,4).map(item => formatSingleMessage(item,locale,catalog,game));
  if (!context.length) return result;
  const translated = result.translated && context.every(item => item.translated);
  const separator = /^(zh|ja)(-|$)/.test(locale) ? '：' : ': ';
  const isolate = (text: string) => /^ar(-|$)/.test(locale) ? `\u2068${text}\u2069` : text;
  return {text:[...context.map(item=>item.text),result.text].map(isolate).join(separator),translated,resolvedLocale:translated ? locale : context.every(item=>item.resolvedLocale==='en') && result.resolvedLocale==='en' ? 'en' : 'mixed'};
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
