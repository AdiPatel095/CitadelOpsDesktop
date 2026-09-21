import { formatMessage, validateMessageCatalog } from './formatMessage';
/** Add explicit descriptors here; never translate user names, IDs, or free-form input. */
export const messages = {
  'locale.select': 'Display language',
  'locale.coverage': 'Official game names use the selected language. Custom interface text is currently English.',
  'metadata.unavailable': 'Official troop and tool metadata is temporarily unavailable. CitadelOps will retry automatically.',
} as const;
export type MessageKey = keyof typeof messages;
export type MessageParameters = Readonly<Record<string, string | number>>;
export type MessageDescriptor = { key: MessageKey; parameters?: MessageParameters };
export type MessageCatalog = Record<MessageKey, string>;
export function validateCatalog(catalog: Record<string, string>): string[] {
  return validateMessageCatalog(messages, catalog);
}
export function interpolate(template: string, parameters: MessageParameters = {}): string {
  return formatMessage({key: '', fallback: template, params: {...parameters}}, 'en', {}).text;
}
