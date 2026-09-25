import { useMemo, useState } from 'react';
import { parseMessageDescriptor } from './messageDescriptor';
import { useLocalizedMessage } from './useLocalizedMessage';
/** Store the original error, not a rendered translation, so locale changes remain reactive. */
export function useLocalizedErrorState(initial = '') {
  const [error,setError] = useState<string | Error>(initial);
  const descriptor = useMemo(()=>error instanceof Error && 'messageDescriptor' in error ? parseMessageDescriptor(error.messageDescriptor) : undefined,[error]);
  const result = useLocalizedMessage(descriptor,error instanceof Error ? error.message : error);
  return [result.text,setError,result] as const;
}
