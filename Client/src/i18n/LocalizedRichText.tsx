import { messageLanguageAttributes } from './messageLanguage';
import { useLocale } from './LocaleContext';
import { richMessages } from './richMessages';
import { renderRichMessage } from './RichMessage';
import type { RichMessageTags } from './RichMessage';
import type { MessageValue } from './formatMessage';
export function LocalizedRichText({messageKey, params, tags}: {messageKey: keyof typeof richMessages; params?: Record<string,MessageValue>; tags: RichMessageTags}) {
  const {locale,catalog} = useLocale();
  const result = renderRichMessage({key:messageKey,fallback:richMessages[messageKey],params},locale,catalog,tags);
  return <span {...messageLanguageAttributes({resolvedLocale:result.translated ? locale : 'en'})}>{result.content}</span>;
}
