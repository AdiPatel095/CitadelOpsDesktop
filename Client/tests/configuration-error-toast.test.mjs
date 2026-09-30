import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {after, test} from 'node:test';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';

const SRC = '../src/';
const vite = await createServer({root: fileURLToPath(new URL('..', import.meta.url)), appType: 'custom', logLevel: 'silent', server: {middlewareMode: true}});
after(() => vite.close());
const load = (path) => vite.ssrLoadModule(fileURLToPath(new URL(`${SRC}${path}`, import.meta.url)));
const {APIError} = await load('api/CitadelClient.ts');
const {shouldToastConfigurationError} = await load('api/configurationErrorToast.ts');

const conflict = () => new APIError('Settings changed.', 409, 'configuration_conflict');

test('a copy conflict explained by the editor does not publish a toast', () => {
  assert.equal(shouldToastConfigurationError(conflict(), true), false);
});

test('configuration conflicts without an editor flag retain their toast', () => {
  assert.equal(shouldToastConfigurationError(conflict()), true);
  assert.equal(shouldToastConfigurationError(conflict(), false), true);
});

test('other API error codes retain their toast even with the editor flag', () => {
  assert.equal(shouldToastConfigurationError(new APIError('Unavailable.', 503, 'unavailable'), true), true);
  assert.equal(shouldToastConfigurationError(new APIError('Other conflict.', 409, 'history_retention_conflict'), true), true);
});

test('non-API errors and undefined retain their toast', () => {
  assert.equal(shouldToastConfigurationError(new Error('Network failed.'), true), true);
  assert.equal(shouldToastConfigurationError({code: 'configuration_conflict'}, true), true);
  assert.equal(shouldToastConfigurationError(undefined, true), true);
});

test('updateConfiguration guards its toast after refreshing and still rethrows the conflict', () => {
  const source = readFileSync(new URL(`${SRC}api/ApiContext.tsx`, import.meta.url), 'utf8');
  assert.match(source, /import \{ shouldToastConfigurationError \} from '\.\/configurationErrorToast';/);
  const options = source.slice(source.indexOf('export type ConfigurationUpdateOptions ='), source.indexOf('const APIContext ='));
  assert.equal((options.match(/conflictShownByEditor\?: boolean/g) ?? []).length, 2);
  const update = source.slice(source.indexOf('const updateConfiguration = useCallback('), source.indexOf('const applyPlayerHistoryRetention = useCallback('));
  assert.match(update, /if \(shouldToastConfigurationError\(requestError, options\?\.conflictShownByEditor\)\) \{\s*Notifications\.error\(errorMessage\(requestError\)\);\s*\}\s*throw requestError;/);
  assert.ok(update.indexOf('acceptConfigurationSnapshot(await CitadelAPI.getConfiguration(configurationScope))') < update.indexOf('if (shouldToastConfigurationError('));
});

test('saveSection marks only a recorded copy as explained by the editor and tracks copyReplay', () => {
  const source = readFileSync(new URL(`${SRC}settings/ConfigurationDraftSession.tsx`, import.meta.url), 'utf8');
  const saveSection = source.slice(source.indexOf('const saveSection = useCallback('), source.indexOf('const save = useCallback('));
  assert.match(saveSection, /updateConfiguration\(\s*targetSection,\s*value,\s*\{\s*\.\.\.configurationDraftSaveCondition\(baseline, targetSection, configurationWideBaseline\),\s*conflictShownByEditor: copyReplay !== undefined,\s*\},\s*\)/);
  assert.match(saveSection, /\}, \[copyReplay, hasConfigurationDependencies, (?:localizeStatic, )?updateConfiguration\]\);/);
});
