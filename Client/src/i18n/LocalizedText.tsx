import { messageLanguageAttributes } from './messageLanguage';
import { useLocale } from './LocaleContext';
import type { MessageKey, MessageParameters } from './messages';
/** A typed static text sink. Keep mixed rich sentences in RichMessage instead. */
export function LocalizedText({ messageKey, params }: { messageKey: MessageKey; params?: MessageParameters }) {
  const { message } = useLocale();
  const result = message(messageKey,params);
  return <span {...messageLanguageAttributes(result)}>{result.text}</span>;
}
