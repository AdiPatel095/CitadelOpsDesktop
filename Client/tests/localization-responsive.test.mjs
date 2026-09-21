import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';
test('responsive world detail navigation uses structural identity, not translated accessible text',()=>{
 const source=fs.readFileSync(new URL('../src/worldIntelligence/components/WorldIntelligenceView.tsx',import.meta.url),'utf8');
 const css=fs.readFileSync(new URL('../src/MaterialExpressive.css',import.meta.url),'utf8');
 assert.match(source,/<nav aria-label=\{localizeStatic\([^\n]+className="world-intelligence-detail-nav sticky/);
 assert.match(css,/\[data-view="world-intelligence"\] \.world-intelligence-detail-nav\s*\{\s*position: static;/);
 assert.doesNotMatch(css,/\[aria-label\s*=/);
});
