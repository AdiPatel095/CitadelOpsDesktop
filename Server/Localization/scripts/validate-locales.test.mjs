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
 assert.match(run({...good,selection:'{mode, select, safe {Behalten} other {Warten}}'}).stderr,/select branches changed/);
 assert.match(run({...good,quantity:'Einheiten behalten'}).stderr,/numeric style changed|argument mismatch/);
 assert.match(run({...good,quantity:'Für {name} {count, number} Einheiten behalten'}).stderr,/numeric style changed/);
 assert.match(run(good,p=>p.entries.de.quantity.sourceSha256='stale').stderr,/stale\/missing provenance/);
 assert.match(run({...good,plural:'{count, plural, one {Eine Einheit}'}).stderr,/Error/);
 console.log('Validator rejects missing arguments, lost precision, stale source hashes, and malformed ICU; accepts locale plural grammar.');
} finally { fs.rmSync(temp,{recursive:true,force:true}); }
