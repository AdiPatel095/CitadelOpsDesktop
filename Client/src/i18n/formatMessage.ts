import { IntlMessageFormat } from 'intl-messageformat';
import { parse, TYPE } from '@formatjs/icu-messageformat-parser';
import type { MessageFormatElement } from '@formatjs/icu-messageformat-parser';
export type LocalizedMessage = {key: string; fallback: string; params?: Record<string, string | number | boolean>};
export type Catalog = Readonly<Record<string,string>>;
/** Plain text only. React renders the result as text, never HTML. */
export function formatMessage(message: LocalizedMessage, locale: string, catalog: Catalog): {text: string; translated: boolean; resolvedLocale: string} {
  const translated = Object.hasOwn(catalog,message.key) && typeof catalog[message.key] === 'string';
  const params = Object.fromEntries(Object.entries(message.params ?? {}).map(([key,value]) => [key, typeof value === 'boolean' ? String(value) : value]));
  const render = (template: string, language: string) => {
    const result = new IntlMessageFormat(template,language,undefined,{ignoreTag:true}).format(params);
    return Array.isArray(result) ? result.join('') : String(result);
  };
  if (translated) {
    try { return {text: render(catalog[message.key],locale),translated:true,resolvedLocale:locale}; }
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
