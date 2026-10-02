import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import IntlMessageFormat from 'intl-messageformat';

const vite = await createServer({
  root: fileURLToPath(new URL('..', import.meta.url)), configFile: false,
  appType: 'custom', logLevel: 'silent', server: { middlewareMode: true },
  plugins: [{ name: 'attack-count-header-contexts', enforce: 'pre',
    resolveId(source, importer) {
      if (!importer?.endsWith('/header/StatusCluster.tsx')) return;
      if (source.endsWith('/ApiContext')) return 'virtual:cit88-api';
      if (source.endsWith('/LocaleContext')) return 'virtual:cit88-locale';
      if (source.endsWith('/Deployment')) return 'virtual:cit88-presence';
    },
    load(id) {
      if (id === 'virtual:cit88-api') return 'export const useCitadelAPI = () => ({state: globalThis.__cit88State});';
      if (id === 'virtual:cit88-locale') return 'export const useLocale = () => globalThis.__cit88Locale;';
      if (id === 'virtual:cit88-presence') return 'export const useHostedRuntimePresence = () => globalThis.__cit88Presence;';
    },
  }],
});
const { rateView, dailyView } = await vite.ssrLoadModule('/src/components/automation/attackCounts.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');
const { attacksText } = await vite.ssrLoadModule('/src/components/header/headerStatus.ts');
after(async () => {
  delete globalThis.__cit88State; delete globalThis.__cit88Locale; delete globalThis.__cit88Presence;
  await vite.close();
});
const observedAt = '2026-10-01T12:00:00Z';
const start = '2026-10-01T11:40:00Z';
const rates = { observedAt, windowMinutes: 60, launchesByFeature: {}, dailySession: { startedAt: start, window: 'since', launchesByFeature: {} } };
const feature = 'autoTowers';
const unknown = {kind: 'unknown'};

test('loading, null and missing maps never default unknown to zero', () => {
  for (const value of [undefined, null, {}, {launchesByFeature: null, dailySession: {startedAt: start}}, {dailySession: {launchesByFeature: null}}]) {
    assert.deepEqual(rateView(value, feature, false), unknown);
    assert.deepEqual(dailyView(value, feature, false), unknown);
  }
  assert.deepEqual(dailyView({...rates, dailySession: null}, feature, false), unknown);
});
test('fresh and empty-restart counts are known zero; missing feature in a present map is zero', () => {
  assert.deepEqual(rateView(rates, feature, false), {kind:'count',count:0,window:'hour'});
  assert.deepEqual(dailyView(rates, feature, false), {kind:'count',count:0,window:'since',since:start});
});
test('mid-day, new-day and restored counts preserve their window and number', () => {
  const daily = {...rates, dailySession: {...rates.dailySession, launchesByFeature:{autoTowers:7}}};
  assert.deepEqual(dailyView(daily, feature, false), {kind:'count',count:7,window:'since',since:start});
  assert.deepEqual(dailyView(JSON.parse(JSON.stringify(daily)), feature, false), dailyView(daily, feature, false));
  assert.deepEqual(dailyView({...daily,dailySession:{...daily.dailySession,window:'day',launchesByFeature:{}}},feature,false), {kind:'count',count:0,window:'day',since:start});
});
test('old runtime daily windows retain the original day meaning', () => {
  const {window: ignored, ...oldSession} = rates.dailySession;
  assert.equal(dailyView({...rates,dailySession:oldSession},feature,false).window,'day');
});
test('hourly partial windows use since, including known zero; full hour stays hour', () => {
  assert.deepEqual(rateView({...rates,windowStartedAt:start},feature,false), {kind:'count',count:0,window:'since',since:start});
  assert.equal(rateView({...rates,windowStartedAt:'2026-10-01T11:00:00Z'},feature,false).window,'hour');
});
test('offline always withholds live counts even while the last live response is cached', () => {
  assert.deepEqual(rateView(rates,feature,true),unknown);
  assert.deepEqual(dailyView(rates,feature,true),unknown);
});

function header(state, presence={mode:'live'}) {
  const locale={locale:'en',messageLocale:'en',number:n=>String(n),t:(key,params)=>String(new IntlMessageFormat(messages[key],'en').format(params))};
  const value=attacksText({dailyAttacks:state?.dailyAttacks,presence},locale);
  return renderToStaticMarkup(React.createElement('span',{title:value.title,'aria-label':value.text==='—'?value.title:value.text},value.text));
}
test('header unknown renders an em dash and the unknown accessible tooltip', () => {
  for (const state of [undefined, {}, {dailyAttacks:{count:0}}, {dailyAttacks:{observedAt,count:undefined}}, {dailyAttacks:{count:0,observedAt:'0001-01-01T00:00:00Z'}}]) {
    const html=header(state);
    assert.match(html,/>—<\/span>/);
    assert.match(html,/title="The attack count isn&#x27;t available right now"/);
    assert.match(html,/aria-label="The attack count isn&#x27;t available right now"/);
    assert.doesNotMatch(html,/>0<\/span>/);
  }
});
test('observed header zero is a real count; checkpoint count only renders its saved form', () => {
  assert.match(header({dailyAttacks:{count:0,observedAt}}),/>0 attacks today<\/span>/);
  const html=header({dailyAttacks:{count:19,observedAt}}, {mode:'checkpoint',checkpointObservedAt:observedAt});
  assert.match(html,/19 attacks, saved /);
  assert.doesNotMatch(html,/today/i);
  assert.match(html,/aria-label="19 attacks, saved /);
});
test('checkpoint without an observed counter stays unknown', () => {
  assert.match(header({}, {mode:'checkpoint',checkpointObservedAt:observedAt}),/>—<\/span>/);
});
