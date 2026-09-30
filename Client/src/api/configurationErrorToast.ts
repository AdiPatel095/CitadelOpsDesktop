import { APIError } from './CitadelClient';

export function shouldToastConfigurationError(error: unknown, conflictShownByEditor?: boolean): boolean {
  return !(conflictShownByEditor === true
    && error instanceof APIError
    && error.code === 'configuration_conflict');
}
