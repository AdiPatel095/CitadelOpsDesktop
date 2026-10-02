import { readFile, readdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

export function areaFor(view) {
  if (/^(landing|account|auth)/.test(view)) return 'a public/account';
  if (['castle', 'automation', 'feature-stats', 'events', 'equipment', 'world-intel', 'world-intelligence'].includes(view)) return 'b core';
  if (['attack-presets', 'defense-presets', 'movement', 'commanders', 'battle-stats', 'player-tracker', 'my-stats', 'alliance-targets', 'rift', 'rift-raid'].includes(view)) return 'c analytics';
  if (/^(settings|patch-notes|support|toast)/.test(view)) return 'd system';
  throw new Error(`Unknown gate view: ${view}`);
}

export function buildReport(reports) {
  const groups = new Map();
  let total = 0;
  const cases = [];
  for (const report of reports) {
    if (!report || typeof report.view !== 'string' || !Number.isInteger(report.width) || !['layout', 'keyboard'].includes(report.suite) || !Array.isArray(report.violations)) {
      throw new Error('Invalid gate report: expected suite, view, integer width and violations array');
    }
    const area = areaFor(report.view);
    cases.push({ suite: report.suite, view: report.view, width: report.width, violations: report.violations.length });
    for (const finding of report.violations) {
      if (!finding || !['rule', 'element', 'detail'].every(key => typeof finding[key] === 'string')) throw new Error(`Invalid violation in ${report.view}`);
      const key = JSON.stringify([area, finding.rule, report.view]);
      const group = groups.get(key) ?? { area, rule: finding.rule, view: report.view, count: 0, widths: [], findings: [] };
      group.count++;
      total++;
      if (!group.widths.includes(report.width)) group.widths.push(report.width);
      group.findings.push({ suite: report.suite, width: report.width, element: finding.element, detail: finding.detail });
      groups.set(key, group);
    }
  }
  const compare = (a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b), 'en');
  const sorted = [...groups.values()].sort((a, b) => compare([a.area, a.rule, a.view], [b.area, b.rule, b.view]));
  for (const group of sorted) {
    group.widths.sort((a, b) => a - b);
    group.findings.sort(compare);
  }
  cases.sort(compare);
  return { schemaVersion: 1, mode: 'report-only', reportCount: reports.length, violationCount: total, cases, groups: sorted };
}

const cell = value => String(value).replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('|', '&#124;').replace(/[\r\n]+/g, ' ');
export function toMarkdown(report) {
  return [
    '# CIT-74 gate report', '',
    `Report-only: ${report.reportCount} case/width reports, ${report.violationCount} findings. Product violations do not fail this prep harness.`, '',
    '| Area | Rule | View | Widths | Findings |',
    '| --- | --- | --- | --- | ---: |',
    ...report.groups.map(group => `| ${cell(group.area)} | ${cell(group.rule)} | ${cell(group.view)} | ${group.widths.join(', ')} | ${group.count} |`),
    ...(report.groups.length ? [] : ['', 'No findings in the supplied reports.']), '',
  ].join('\n');
}

export async function generateReport(input = 'test-results/gate', output = '.') {
  const names = (await readdir(input)).filter(name => name.endsWith('.json') && name !== 'gate-report.json').sort();
  if (!names.length) throw new Error(`No gate JSON reports in ${input}`);
  const reports = [];
  for (const name of names) {
    try { reports.push(JSON.parse(await readFile(resolve(input, name), 'utf8'))); }
    catch (error) { throw new Error(`Cannot read gate input ${name}: ${error.message}`, { cause: error }); }
  }
  const report = buildReport(reports);
  await writeFile(resolve(output, 'gate-report.json'), `${JSON.stringify(report, null, 2)}\n`);
  await writeFile(resolve(output, 'gate-report.md'), toMarkdown(report));
  return report;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const report = await generateReport(process.argv[2], process.argv[3]);
  console.log(`${report.reportCount} reports, ${report.violationCount} findings (report-only)`);
}
