import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
const parser=process.argv[2];
if(!parser) throw new Error('Pass the installed FormatJS parser module path');
const sourceDir=path.dirname(fileURLToPath(import.meta.url));
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'cit-locale-validator-'));
const sha=x=>createHash('sha256').update(x).digest('hex');
try {
 fs.mkdirSync(path.join(temp,'scripts'));fs.mkdirSync(path.join(temp,'locales'));
 fs.copyFileSync(path.join(sourceDir,'validate-locales.mjs'),path.join(temp,'scripts/validate-locales.mjs'));
 const locales=['de','fr','pl','ru','it','nl','pt','es','ar','da','no','fi','sv','ja','ko','el','tr','zh-CN','zh-TW','cs','ro','sk','hu','bg','lt'];
 for(const locale of locales) fs.writeFileSync(path.join(temp,'locales',locale+'.json'),'{}');
 const english={quantity:'Keep {count, number, ::precision-integer} units for {name}',plural:'Keep {count} units',selection:'{mode, select, safe {Keep} stop {Stop} other {Wait}}'};
 fs.writeFileSync(path.join(temp,'en.json'),JSON.stringify(english));
 function run(pack,mutate=()=>{}) {
  const provenance={entries:{de:Object.fromEntries(Object.entries(pack).map(([k,v])=>[k,{method:'agent-authored',sourceSha256:sha(english[k]??''),translationSha256:sha(v)}]))}};
  mutate(provenance);
  fs.writeFileSync(path.join(temp,'locales/de.json'),JSON.stringify(pack));fs.writeFileSync(path.join(temp,'locales/provenance.json'),JSON.stringify(provenance));
  return spawnSync(process.execPath,[path.join(temp,'scripts/validate-locales.mjs'),path.resolve(parser)],{encoding:'utf8'});
 }
 const good={quantity:'Für {name} {count, number, ::precision-integer} Einheiten behalten',plural:'{count, plural, one {Eine Einheit behalten} other {# Einheiten behalten}}',selection:'{mode, select, safe {Behalten} stop {Stoppen} other {Warten}}'};
 assert.equal(run(good).status,0,'locale plural grammar should be accepted');
 assert.match(run(good,p=>p.sourceCatalogSha256='stale').stderr,/Source catalog hash changed/);
 assert.match(run({...good,selection:'{mode, select, safe {Behalten} other {Warten}}'}).stderr,/select branches changed/);
 assert.match(run({...good,quantity:'Einheiten behalten'}).stderr,/numeric style changed|argument mismatch/);
 assert.match(run({...good,quantity:'Für {name} {count, number} Einheiten behalten'}).stderr,/numeric style changed/);
 assert.match(run(good,p=>p.entries.de.quantity.sourceSha256='stale').stderr,/stale\/missing provenance/);
 assert.match(run({...good,plural:'{count, plural, one {Eine Einheit}'}).stderr,/Error/);
 english.duplicate=english.quantity;
 fs.writeFileSync(path.join(temp,'en.json'),JSON.stringify(english));
 const reused={...good,duplicate:good.quantity};
 function withReuse(p) {
  Object.assign(p.entries.de.duplicate,{method:'reviewed-source-key-reuse',translationSourceKey:'quantity',originRevision:'a'.repeat(40),originSourceSha256:sha(english.quantity),originTranslationSha256:sha(good.quantity),semanticReview:'Same fixture meaning and argument contract.'});
 }
 assert.equal(run(reused,withReuse).status,0,'reviewed exact source-key reuse should pass');
 assert.match(run(reused,p=>{withReuse(p);p.entries.de.duplicate.translationSourceKey='plural'}).stderr,/invalid reviewed source-key reuse/);
 assert.match(run(reused,p=>{withReuse(p);p.entries.de.duplicate.originTranslationSha256='stale'}).stderr,/invalid reviewed source-key reuse/);
 assert.match(run(reused,p=>{withReuse(p);delete p.entries.de.duplicate.semanticReview}).stderr,/invalid reviewed source-key reuse/);
 assert.match(run({...reused,duplicate:'{count, number, ::precision-integer} Einheiten für {name}'},withReuse).stderr,/invalid reviewed source-key reuse/);
 const featureKey='server.telemetry.channel.example.label';
 english[featureKey]='Example feature';
 fs.writeFileSync(path.join(temp,'en.json'),JSON.stringify(english));
 const featureGlossary={features:{example:{source:english[featureKey],translations:Object.fromEntries(locales.map(locale=>[locale,'Beispielfunktion']))}}};
 const glossaryBytes=JSON.stringify(featureGlossary);
 fs.writeFileSync(path.join(temp,'feature-names.json'),glossaryBytes);
 for(const locale of locales.filter(locale=>locale!=='de')) fs.writeFileSync(path.join(temp,'locales',locale+'.json'),JSON.stringify({[featureKey]:'Beispielfunktion'}));
 function withGlossary(p){
  p.featureGlossary={sha256:sha(glossaryBytes)};
  for(const locale of locales){
   p.entries[locale]??={};
   p.entries[locale][featureKey]={sourceSha256:sha(english[featureKey]),translationSha256:sha('Beispielfunktion'),glossaryFeature:'example'};
  }
 }
 const glossaryGood={...good,[featureKey]:'Beispielfunktion'};
 assert.equal(run(glossaryGood,withGlossary).status,0,'matching glossary labels should pass');
 assert.match(run({...glossaryGood,[featureKey]:'Different label'},p=>{withGlossary(p);p.entries.de[featureKey].translationSha256=sha('Different label')}).stderr,/feature label drift/);
 assert.match(run(glossaryGood,p=>{withGlossary(p);p.featureGlossary.sha256='stale'}).stderr,/glossary source hash changed/);
 console.log('Validator rejects missing arguments, lost precision, stale source hashes, and malformed ICU; accepts locale plural grammar.');
} finally { fs.rmSync(temp,{recursive:true,force:true}); }
