export { AUTOMATION_FEATURE_NAMES } from '../../src/settings/automationFeatureNames';
import { prepare } from '../visual/harness';
import type { Page } from '@playwright/test';
import type { GateCase } from './views';
import type { GateOptions } from './harness';
export const isDesktop = true;
export function prepareCase(page: Page, entry: GateCase, options: GateOptions) {
  return prepare(page, options.theme, entry.states, entry.scenario, options.locale, entry.toast === 'failure' ? 'start' : '', entry.toast === 'success');
}
