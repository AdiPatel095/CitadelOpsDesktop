import fs from 'node:fs';
import { sourceMessages } from '../src/i18n/sourceMessages.ts';
import ts from 'typescript';
import { validateMessageCatalog } from '../src/i18n/formatMessage.ts';
import { officialMessageKeys } from '../src/i18n/officialKeys.ts';
import { localeCodes } from '../src/i18n/locales.ts';
const source = fs.readFileSync(new URL('../src/i18n/messages.ts',import.meta.url),'utf8');
const ast = ts.createSourceFile('messages.ts',source,ts.ScriptTarget.Latest,true);
let messages = {...sourceMessages};
function visit(node) {
  if (ts.isVariableDeclaration(node) && node.name.getText(ast) === 'messages') messages = {...messages,...Object.fromEntries(node.initializer.expression.properties.filter(ts.isPropertyAssignment).map(property => [property.name.text,property.initializer.text]))};
  ts.forEachChild(node,visit);
}
visit(ast);
const custom = Object.fromEntries(Object.entries(messages).filter(([key]) => !Object.hasOwn(officialMessageKeys,key)));
fs.writeFileSync(new URL('../localization/ui.en.json',import.meta.url),JSON.stringify(custom,null,2)+'\n');
const errors = [];
const missingByLocale = {};
for (const locale of localeCodes.filter(code=>code!=='en')) {
  const path = new URL(`../src/i18n/catalogs/${locale}.json`,import.meta.url);
  if (!fs.existsSync(path)) { errors.push(`${locale}: missing catalog`); continue; }
  const catalog=JSON.parse(fs.readFileSync(path,'utf8'));
  missingByLocale[locale]=Object.keys(custom).filter(key=>!Object.hasOwn(catalog,key)).length;
  errors.push(...validateMessageCatalog(custom,catalog).map(error=>`${locale}: ${error}`));
}
const inventory = JSON.parse(fs.readFileSync(new URL('../localization/source-inventory.json',import.meta.url),'utf8'));
const sourceOpen = inventory.entries.filter(entry=>entry.classification==='unreviewed').length;
const dynamicGaps = JSON.parse(fs.readFileSync(new URL('../localization/dynamic-gaps.json',import.meta.url),'utf8'));
console.log(JSON.stringify({typedKeys:Object.keys(messages).length,officialKeys:Object.keys(officialMessageKeys).length,customKeys:Object.keys(custom).length,authoredLocales:localeCodes.length,missingByLocale,catalogErrorCount:errors.length,catalogErrors:errors.filter(error=>!error.includes('Missing message:')),unreviewedSourceCandidates:sourceOpen,openDynamicSurfaces:dynamicGaps.filter(gap=>gap.status!=='covered').map(gap=>gap.id)},null,2));
// This release gate intentionally remains red until semantic source review and dynamic coverage are complete.
process.exitCode = errors.length || sourceOpen || dynamicGaps.some(gap=>gap.status!=='covered') ? 1 : 0;
