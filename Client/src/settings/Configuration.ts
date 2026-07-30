import type { ConfigurationSnapshot } from '../api/Contracts';
import { CitadelAPI } from '../api/CitadelClient';

export function configurationSection(
  configuration: ConfigurationSnapshot | null,
  section: string,
): Record<string, unknown> {
  return asRecord(configuration?.sections[section]);
}

export function asRecord(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {};
}

export function numericSetting(value: unknown, fallback: number): number {
  const number = Number(value);
  return Number.isFinite(number) ? number : fallback;
}

export function queueConfigurationUpdate(section: string, value: unknown): boolean {
  void CitadelAPI.submitIntent(
    'config.update',
    { section, value },
    { actor: 'ui-settings' },
  ).catch((error) => {
    console.error(`Could not save ${section} configuration`, error);
  });
  return true;
}
