import fs from 'node:fs';
import path from 'node:path';
import {pathToFileURL, fileURLToPath} from 'node:url';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const parserPath = process.argv[2];
if (!parserPath) throw new Error('Usage: node validate-locales.mjs /path/to/@formatjs/icu-messageformat-parser/index.js');
const {parse} = await import(pathToFileURL(path.resolve(parserPath)).href);
const english = JSON.parse(fs.readFileSync(path.join(root, 'en.json'), 'utf8'));
const provenance = JSON.parse(fs.readFileSync(path.join(root, 'locales/provenance.json'), 'utf8'));
const expectedLocales = ['en','de','fr','pl','ru','it','nl','pt','es','ar','da','no','fi','sv','ja','ko','el','tr','zh-CN','zh-TW','cs','ro','sk','hu','bg','lt'].filter(x=>x!=='en');
function argumentsOf(nodes, result = new Set()) {
 for (const node of nodes) {
  if ([1,2,3,4,5,6,8].includes(node.type)) result.add(node.value);
  if (node.options) for (const option of Object.values(node.options)) argumentsOf(option.value, result);
  if (node.children) argumentsOf(node.children, result);
 }
 return [...result].sort();
}
function selectBranches(nodes, result = new Map()) {
 for (const node of nodes) {
  if (node.type===5) {
   const branches=result.get(node.value)??new Set();
   for(const branch of Object.keys(node.options)) branches.add(branch);
   result.set(node.value,branches);
  }
  if (node.options) for(const option of Object.values(node.options)) selectBranches(option.value,result);
  if (node.children) selectBranches(node.children,result);
 }
 return result;
}
function numberStyles(nodes, result = new Map()) {
 for (const node of nodes) {
  if (node.type===2 && node.style) {
   const styles=result.get(node.value)??new Set();
   styles.add(JSON.stringify(typeof node.style==='object' ? node.style.tokens : node.style));result.set(node.value,styles);
  }
  if (node.options) for(const option of Object.values(node.options)) numberStyles(option.value,result);
  if (node.children) numberStyles(node.children,result);
 }
 return result;
}
const sourceAST=Object.fromEntries(Object.entries(english).map(([key,text])=>[key,parse(text)]));
const {createHash} = await import('node:crypto');
const hash = value => createHash('sha256').update(value).digest('hex');
const glossaryPath=path.join(root,'feature-names.json');
const glossaryBytes=fs.existsSync(glossaryPath)?fs.readFileSync(glossaryPath):null;
if(provenance.featureGlossary && (!glossaryBytes || provenance.featureGlossary.sha256!==hash(glossaryBytes))) throw new Error('Feature glossary source hash changed');
const glossary=glossaryBytes?JSON.parse(glossaryBytes):null;
if(glossary && !provenance.featureGlossary) throw new Error('Feature glossary provenance missing');
let total = 0;
const coverage = {};
for (const locale of expectedLocales) {
 const pack = JSON.parse(fs.readFileSync(path.join(root, 'locales', locale+'.json'), 'utf8'));
 for (const [key,text] of Object.entries(pack)) {
  if (!(key in english)) throw new Error(`${locale}: unknown key ${key}`);
  if (typeof text !== 'string' || !text.trim()) throw new Error(`${locale}: empty translation ${key}`);
  const sourceArgs=argumentsOf(sourceAST[key]);
  const translatedAST=parse(text);
  const translatedArgs=argumentsOf(translatedAST);
  const sourceSelects=selectBranches(sourceAST[key]);
  const targetSelects=selectBranches(translatedAST);
  for(const [name,branches] of sourceSelects) if(JSON.stringify([...branches].sort())!==JSON.stringify([...(targetSelects.get(name)??[])].sort())) throw new Error(`${locale}: select branches changed for ${name} in ${key}`);
  const sourceStyles=numberStyles(sourceAST[key]);
  const targetStyles=numberStyles(translatedAST);
  for(const [name,styles] of sourceStyles) for(const style of styles) if(!targetStyles.get(name)?.has(style)) throw new Error(`${locale}: numeric style changed for ${name} in ${key}`);
  if (JSON.stringify(sourceArgs)!==JSON.stringify(translatedArgs)) throw new Error(`${locale}: argument mismatch ${key}`);
  const record=provenance.entries[locale]?.[key];
  if (!record || record.sourceSha256!==hash(english[key]) || record.translationSha256!==hash(text)) throw new Error(`${locale}: stale/missing provenance ${key}`);
  total++;
 }
 for(const [feature,entry] of Object.entries(glossary?.features??{})) {
  const key=`server.telemetry.channel.${feature}.label`;
  if(english[key]!==entry.source) throw new Error(`Feature glossary English source changed: ${feature}`);
  if(typeof entry.translations?.[locale]!=='string' || pack[key]!==entry.translations[locale]) throw new Error(`${locale}: feature label drift ${feature}`);
  if(provenance.entries[locale]?.[key]?.glossaryFeature!==feature) throw new Error(`${locale}: feature glossary provenance missing ${feature}`);
 }
 for(const key of Object.keys(provenance.entries[locale]??{})) if(!(key in pack)) throw new Error(`${locale}: obsolete provenance ${key}`);
 coverage[locale]={translated:Object.keys(pack).length,source:Object.keys(english).length,missing:Object.keys(english).length-Object.keys(pack).length};
}
console.log(JSON.stringify({validated:total,coverage},null,2));
