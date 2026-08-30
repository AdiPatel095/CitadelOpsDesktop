import type { ConfigurationSnapshot } from '../api/Contracts';
import { APIError, CitadelAPI, type ConfigurationUpdateCondition } from '../api/CitadelClient';

let legacyConfigurationQueue: Promise<void> = Promise.resolve();

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

export function queueConfigurationUpdate(section: string, value: unknown): Promise<ConfigurationSnapshot> {
	const configurationScope = CitadelAPI.configurationScope();
  const save = async (): Promise<ConfigurationSnapshot> => {
    const update = async () => {
      const snapshot = await CitadelAPI.getConfiguration(configurationScope);
      const current = snapshot.sections[section];
      const condition: ConfigurationUpdateCondition = current === undefined
        ? { expectedRevision: snapshot.revision }
        : { expectedValue: current };
      return CitadelAPI.updateConfiguration(section, value, condition, configurationScope);
    };
    try {
      return await update();
    } catch (error) {
      if (!(error instanceof APIError) || error.code !== 'configuration_conflict') throw error;
      return update();
    }
  };
  const result = legacyConfigurationQueue.then(save, save);
  legacyConfigurationQueue = result.then(() => undefined, () => undefined);
  return result;
}
