import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {readExternalCatalogSource,assertServerCatalogSynchronized} from '../scripts/external-catalog-source.mjs';
for(const family of ['server','backend'])test(`external ${family} coverage rejects absent, empty and unproven sources`,()=>{
 const temp=fs.mkdtempSync(path.join(os.tmpdir(),'cit-locale-source-'));
 try {
  assert.throws(()=>readExternalCatalogSource(temp,family),/missing en.json/);
  for(const invalid of [{}, 'abc', 42, true, null, []]) {
   fs.writeFileSync(path.join(temp,'en.json'),JSON.stringify(invalid));
   assert.throws(()=>readExternalCatalogSource(temp,family),/empty or invalid source/);
  }
  fs.writeFileSync(path.join(temp,'en.json'),'{"key":"Valid message"}');
  assert.throws(()=>readExternalCatalogSource(temp,family),/missing provenance/);
  fs.writeFileSync(path.join(temp,'provenance.json'),'{}');
  assert.throws(()=>readExternalCatalogSource(temp,family),/provenance/);
  const current=fileURLToPath(new URL(`../src/i18n/${family}`,import.meta.url));
  assert.ok(Object.keys(readExternalCatalogSource(current,family)).length>0);
 } finally {fs.rmSync(temp,{recursive:true,force:true});}
});

test('every server catalog copy must exist, match source bytes, and have no extra JSON files',()=>{
 const temp=fs.mkdtempSync(path.join(os.tmpdir(),'cit-locale-runtime-'));
 try {
  const serverRoot=path.join(temp,'server');
  const client=path.join(temp,'client');
  fs.mkdirSync(path.join(serverRoot,'locales'),{recursive:true});
  fs.mkdirSync(client);
  const check=()=>assertServerCatalogSynchronized(client,serverRoot);
  assert.throws(check,/missing/);
  fs.writeFileSync(path.join(serverRoot,'en.json'),'{}');assert.throws(check,/invalid/);
  const files={'en.json':' {"key":"Message"}', 'feature-names.json':'{"features":{}}', 'provenance.json':'{"sourceRevision":"test","entries":{"de":{}}}', 'de.json':'{"key":"Nachricht"}'};
  for(const [name,bytes] of Object.entries(files)) {
   fs.writeFileSync(path.join(serverRoot,name==='en.json'||name==='feature-names.json'?'':'locales',name),bytes);
   fs.writeFileSync(path.join(client,name),bytes);
  }
  assert.doesNotThrow(check);
  for(const [name,bytes] of Object.entries(files)) {
   fs.writeFileSync(path.join(client,name),bytes+'\n');
   assert.throws(check,/does not match/,`${name} must match byte-for-byte`);
   fs.unlinkSync(path.join(client,name));
   assert.throws(check,/missing client catalog/,`${name} must exist`);
   fs.writeFileSync(path.join(client,name),bytes);
  }
  fs.writeFileSync(path.join(client,'extra.json'),'{}');assert.throws(check,/extra client catalog/);
  fs.unlinkSync(path.join(client,'extra.json'));
  fs.mkdirSync(path.join(client,'extra'));
  fs.writeFileSync(path.join(client,'extra','nested.json'),'{}');assert.throws(check,/extra client catalog/);
  fs.rmSync(path.join(client,'extra'),{recursive:true});
  fs.unlinkSync(path.join(serverRoot,'feature-names.json'));
  assert.throws(check,/extra client catalog/);
  fs.unlinkSync(path.join(client,'feature-names.json'));
  assert.doesNotThrow(check,'glossary copy is optional only when its source is absent');
 }finally{fs.rmSync(temp,{recursive:true,force:true});}
});

test('the real checkout has exactly the byte-identical server catalog copies',()=>{
 assertServerCatalogSynchronized(fileURLToPath(new URL('../src/i18n/server',import.meta.url)),fileURLToPath(new URL('../../Server/Localization',import.meta.url)));
});
