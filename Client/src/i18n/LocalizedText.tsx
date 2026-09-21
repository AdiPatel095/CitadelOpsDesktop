import { useLocale } from './LocaleContext';
import type { MessageKey } from './messages';
/** A typed static text sink. Keep mixed rich sentences in RichMessage instead. */
export function LocalizedText({ messageKey }: { messageKey: MessageKey }) {
  const { message } = useLocale();
  const result = message(messageKey);
  return <span lang={result.resolvedLocale === 'mixed' ? undefined : result.resolvedLocale} dir="auto">{result.text}</span>;
}
