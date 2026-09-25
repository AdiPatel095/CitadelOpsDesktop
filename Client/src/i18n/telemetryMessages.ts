import { parseMessageDescriptor } from './messageDescriptor';
import type { LocalizedMessage } from './formatMessage';
export type TelemetryEntry = { raw: string; messageDescriptor?: LocalizedMessage };
/** Only trust descriptors aligned with the exact legacy line, never infer them from prose. */
export function telemetryEntries(value: unknown): TelemetryEntry[] {
  if (!value || typeof value !== 'object') return [];
  const data = value as { lines?: unknown; entries?: unknown };
  if (!Array.isArray(data.lines)) return [];
  const entries = Array.isArray(data.entries) ? data.entries : [];
  return data.lines.flatMap((line, index) => {
    if (typeof line !== 'string') return [];
    const entry = entries[index];
    const descriptor = entry && typeof entry === 'object' && entry.line === line
      ? parseMessageDescriptor(entry.messageDescriptor) : undefined;
    return [{ raw: line, messageDescriptor: descriptor }];
  });
}
