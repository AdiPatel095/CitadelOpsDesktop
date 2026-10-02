import { spawnSync } from 'node:child_process';
import { mkdirSync, rmSync } from 'node:fs';
import { generateReport } from './gate-report.mjs';

// Clear only this suite's generated findings: stale reports cannot count as coverage.
rmSync('test-results/gate', { recursive: true, force: true });
mkdirSync('test-results/gate', { recursive: true });
const result = spawnSync(process.execPath, ['node_modules/@playwright/test/cli.js', 'test', '-c', 'playwright.gate.config.ts', ...process.argv.slice(2)], { stdio: 'inherit' });
if (result.error) throw result.error;
const report = await generateReport('test-results/gate', 'test-results/gate');
console.log(`CIT-74: ${report.reportCount} reports; ${report.violationCount} findings (area enforcement in tests/gate/enforcement.json).`);
process.exitCode = result.status ?? 1;
