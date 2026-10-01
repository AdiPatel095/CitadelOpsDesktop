import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import '../../src/index.css';
import '../../src/MaterialExpressive.css';
import './fixture.css';
import { Dock, SIMULATION_BANNER } from './dock';
import { FixtureServer } from './fixtureServer';
import { installFixtureTransport } from './install';
import { loadScenarioFiles, validateScenarioFile, type SessionMode } from './scenario';

declare const __CANDIDATE_SHA__: string;

/**
 * Onboarding preview entry (CIT-22): installs the fixture transport BEFORE the production application loads, so the
 * unchanged production client talks to fixtures only. Query: ?scenario=<id>&session=<mode>&fail=<stop|start>&locale=<en|de|ar>
 * &account=<1|2>&draft=changed&reset=1
 */
const params = new URLSearchParams(window.location.search);
const scenarios = loadScenarioFiles('desktop');
const problems = scenarios.flatMap(validateScenarioFile);
if (problems.length > 0) console.error('Scenario problems:', problems);
const file = scenarios.find((entry) => entry.id === params.get('scenario')) ?? scenarios.find((entry) => entry.id === 'new-user') ?? scenarios[0];

const clearPreviewStorage = () => {
  try {
    for (const key of Object.keys(window.localStorage)) if (key.startsWith('citadelops')) window.localStorage.removeItem(key);
    for (const key of Object.keys(window.sessionStorage)) if (key.startsWith('citadelops')) window.sessionStorage.removeItem(key);
  } catch { /* storage unavailable: the scenario starts as loaded */ }
};
if (params.get('reset') === '1') {
  clearPreviewStorage();
  params.delete('reset');
  window.history.replaceState(null, '', `${window.location.pathname}?${params.toString()}`);
}

const session = params.get('session');
const server = new FixtureServer({
  file,
  session: session === 'disconnected' || session === 'awaiting-baseline' || session === 'checkpoint' || session === 'live' ? session as SessionMode : undefined,
  fail: params.get('fail') === 'stop' || params.get('fail') === 'start' ? params.get('fail') as 'stop' | 'start' : undefined,
  account: params.get('account') === '2' ? 2 : 1,
});

// Seed the browser-storage state the scenario declares (a recorded draft, a chosen goal, a turn-on time).
for (const seed of server.built.storage) {
  let value = seed.value;
  if (params.get('draft') === 'changed' && seed.key.includes('.draft.v1.')) {
    const entry = JSON.parse(value) as { baseDigest?: string };
    entry.baseDigest = '0:changed';
    value = JSON.stringify(entry);
  }
  try { window.localStorage.setItem(seed.key, value); } catch { /* storage unavailable: no recovered draft */ }
}

installFixtureTransport(server);

const { setViewerLocale } = await import('../../src/i18n/viewerLocaleStore');
const locale = params.get('locale') ?? file.locale ?? 'en';
if (locale === 'de' || locale === 'ar' || locale === 'en') setViewerLocale(locale);

const { default: App } = await import('../../src/App.tsx');
const { LocaleProvider } = await import('../../src/i18n/LocaleContext');

const go = (next: Record<string, string | null>) => {
  const target = new URLSearchParams(window.location.search);
  for (const [key, value] of Object.entries(next)) { if (value === null) target.delete(key); else target.set(key, value); }
  window.location.search = target.toString();
};

document.getElementById('fixture-banner')!.textContent = SIMULATION_BANNER;
createRoot(document.getElementById('fixture-dock')!).render(
  <Dock server={server} scenarios={scenarios} candidate={__CANDIDATE_SHA__} onReset={() => go({ reset: '1' })} onScenario={(id) => go({ scenario: id, reset: '1' })} />,
);
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <LocaleProvider><App /></LocaleProvider>
  </StrictMode>,
);
