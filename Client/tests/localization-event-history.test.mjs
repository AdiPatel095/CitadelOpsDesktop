import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import ts from 'typescript';
const source=fs.readFileSync(new URL('../src/worldIntelligence/components/WorldEventHistory.tsx',import.meta.url),'utf8');
const ast=ts.createSourceFile('WorldEventHistory.tsx',source,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX);
const names=['formatDate','formatDateTime','playerLevelRange'];
const body=ast.statements.filter(node=>ts.isFunctionDeclaration(node)&&names.includes(node.name?.text)).map(node=>node.getText(ast)).join('\n');
const compiled=ts.transpile(body+'\nexport {formatDate,formatDateTime,playerLevelRange};',{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ES2022});
const helpers=await import('data:text/javascript;base64,'+Buffer.from(compiled).toString('base64'));
test('date-only history remains UTC while using selected locale',()=>{
 const sourceDate='2026-09-01';
 for(const locale of ['en','de','ar'])assert.equal(helpers.formatDate(sourceDate,locale,()=>'?'),new Intl.DateTimeFormat(locale,{dateStyle:'medium',timeZone:'UTC'}).format(new Date(sourceDate+'T00:00:00Z')));
 assert.equal(helpers.formatDate('', 'de',()=> 'Unbekannt'),'Unbekannt');
 assert.equal(helpers.formatDate('historic literal', 'de',()=>'?'),'historic literal');
});
test('level league thresholds and canonical numbers survive display localization',()=>{
 const t=(key,params)=>({key,params});
 assert.deepEqual(helpers.playerLevelRange(15,15,t),{key:'events.level',params:{level:15}});
 assert.deepEqual(helpers.playerLevelRange(60,72,t),{key:'events.levelsMixed',params:{minimum:60,maximum:2}});
 assert.deepEqual(helpers.playerLevelRange(70,80,t),{key:'events.legendaryLevels',params:{minimum:0,maximum:10}});
 assert.deepEqual(helpers.playerLevelRange(80,80,t),{key:'events.legendaryLevel',params:{level:10}});
});
