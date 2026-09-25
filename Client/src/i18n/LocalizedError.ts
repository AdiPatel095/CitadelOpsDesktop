import { messages } from './messages';
import type { MessageKey, MessageParameters } from './messages';
import { formatMessage } from './formatMessage';
import type { LocalizedMessage } from './formatMessage';
/** Application validation retains a descriptor; callers never store a translated string. */
export class LocalizedError extends Error {
  readonly messageDescriptor: LocalizedMessage;
  constructor(key:MessageKey,params?:MessageParameters,cause?:unknown) {
    const descriptor={key,fallback:messages[key],params:params ? {...params} : undefined};
    super(formatMessage(descriptor,'en',{}).text,{cause});
    this.name='LocalizedError';this.messageDescriptor=descriptor;
  }
}
