import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import ts from 'typescript';
// Compile the actual pure presentation helper chain; the React screen remains private.
const source=fs.readFileSync(new URL('../src/battleStats/components/BattleStatsView.tsx',import.meta.url),'utf8');
const ast=ts.createSourceFile('BattleStatsView.tsx',source,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX);
const names=new Set(['effectComparisonGroups','addOfficialEffectBucket','sortOfficialGroupEffects','officialEffectCategory','officialEffectGroup','numericValue','stringValue','effectLabel','effectDescription','effectDisplayText','effectValue','categoryDisplayLabel']);
const functions=ast.statements.filter(node=>ts.isFunctionDeclaration(node)&&names.has(node.name?.text)).map(node=>node.getText(ast)).join('\n');
assert.equal(ast.statements.filter(node=>ts.isFunctionDeclaration(node)&&names.has(node.name?.text)).length,names.size);
const compiled=ts.transpile(functions+'\nexport {effectComparisonGroups};',{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ES2022});
const {effectComparisonGroups}=await import('data:text/javascript;base64,'+Buffer.from(compiled).toString('base64'));
const localize=key=>key;
test('distinct unknown legacy categories survive bucket and category grouping',()=>{
 const effects=['Historic alpha','Historic beta'].map(category=>({category,label:'Original',formattedValue:'1%',officialCategory:99,officialGroup:999,officialGroupKey:'official-group-99-999'}));
 const groups=effectComparisonGroups(effects,[],localize);
 assert.equal(groups.length,2);
 assert.deepEqual(groups.map(group=>group.category),['Historic alpha','Historic beta']);
 assert.notEqual(groups[0].key,groups[1].key);
});
test('authoritative categories group by numeric identity despite display language',()=>{
 const effects=['Attack effects','Angriffseffekte'].map(category=>({category,label:'Original',formattedValue:'1%',officialCategory:3,officialGroup:1,officialGroupKey:'official-group-3-1'}));
 const groups=effectComparisonGroups(effects,[],localize);
 assert.equal(groups.length,1);
 assert.equal(groups[0].rows[0].commander.length,2);
 assert.equal(groups[0].key,'3');
});
