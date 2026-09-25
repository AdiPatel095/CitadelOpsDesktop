import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {officialMessageKeys,officialMessageNouns} from '../src/i18n/officialKeys.ts';
import {localeCodes,locales} from '../src/i18n/locales.ts';
const [directory]=process.argv.slice(2);
if(!directory)throw new Error('Pass the directory containing verified v4357 official dictionaries');
const hash=value=>createHash('sha256').update(value).digest('hex');
const keys=[...new Set([...Object.values(officialMessageKeys),...Object.values(officialMessageNouns).flatMap(nouns=>Object.values(nouns).map(noun=>noun.key))])].sort();
const provenance={source:'https://langserv.public.ggs-ep.com/12@{version}/{gameCode}/*',version:'4357',purpose:'Explicit reviewed UI routes available before runtime connection',keys,locales:{}};
const outputs=[];
for(const locale of localeCodes) {
 const raw=fs.readFileSync(path.join(directory,locale==='en'?'en-v4357.json':`${locale}.json`));
 const dictionary=JSON.parse(raw);
 const subset={};
 for(const key of keys) {
  if(typeof dictionary[key]!=='string'||!dictionary[key].trim())throw new Error(`Missing official ${locale}:${key}`);
  subset[key]=dictionary[key];
 }
 const bytes=JSON.stringify(subset,null,2)+'\n';
 outputs.push([new URL(`../src/i18n/officialBundled/${locale}.json`,import.meta.url),bytes]);
 provenance.locales[locale]={gameCode:locales.find(item=>item.code===locale).gameCode,sourceSha256:hash(raw),subsetSha256:hash(bytes),entries:keys.length};
}
for(const [target,bytes] of outputs)fs.writeFileSync(target,bytes);
fs.writeFileSync(new URL('../localization/bundled-official-provenance.json',import.meta.url),JSON.stringify(provenance,null,2)+'\n');
console.log(`Bundled ${keys.length} explicit official keys for ${localeCodes.length} locales`);
