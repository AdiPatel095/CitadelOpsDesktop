import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import ts from 'typescript';
import {pathToFileURL} from 'node:url';
const modules={};
for(const file of ['config/PatchNotes','i18n/sourceMessages','i18n/formatMessage']) {
 const output=new URL(`../node_modules/.patch-${file.replace('/','-')}.mjs`,import.meta.url);
 fs.writeFileSync(output,ts.transpileModule(fs.readFileSync(new URL(`../src/${file}.ts`,import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ES2022,target:ts.ScriptTarget.ES2022}}).outputText);
 modules[file]=await import(pathToFileURL(output.pathname));fs.unlinkSync(output);
}
test('every release subtitle and note has an exact source-preserving typed catalog assignment',()=>{
 const {PATCH_NOTES_RELEASES}=modules['config/PatchNotes'];
 const {sourceMessages}=modules['i18n/sourceMessages'];
 const {formatMessage,messageArguments}=modules['i18n/formatMessage'];
 let count=0;
 for(const release of PATCH_NOTES_RELEASES) {
  for(const [key,text] of [...(release.subtitle?[[release.subtitleKey,release.subtitle]]:[]),...release.items.map(item=>[item.textKey,item.text])]) {
   assert.ok(key&&sourceMessages[key],`${release.version}: missing key`);
   assert.deepEqual(messageArguments(sourceMessages[key]),[]);
   assert.equal(formatMessage({key,fallback:sourceMessages[key]},'en',{}).text,text);
   count++;
  }
 }
 assert.equal(count,263);
});
