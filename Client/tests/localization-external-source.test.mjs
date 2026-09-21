import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {readExternalCatalogSource} from '../scripts/external-catalog-source.mjs';
for(const family of ['server','backend'])test(`external ${family} coverage rejects absent, empty and unproven sources`,()=>{
 const temp=fs.mkdtempSync(path.join(os.tmpdir(),'cit-locale-source-'));
 try {
  assert.throws(()=>readExternalCatalogSource(temp,family),/missing en.json/);
  fs.writeFileSync(path.join(temp,'en.json'),'{}');
  assert.throws(()=>readExternalCatalogSource(temp,family),/empty or invalid source/);
  fs.writeFileSync(path.join(temp,'en.json'),'{"key":"Valid message"}');
  assert.throws(()=>readExternalCatalogSource(temp,family),/missing provenance/);
  fs.writeFileSync(path.join(temp,'provenance.json'),'{}');
  if(family==='server')fs.writeFileSync(path.join(temp,'coverage.json'),'{}');
  assert.throws(()=>readExternalCatalogSource(temp,family),/provenance/);
  const current=fileURLToPath(new URL(`../src/i18n/${family}`,import.meta.url));
  assert.ok(Object.keys(readExternalCatalogSource(current,family)).length>0);
 } finally {fs.rmSync(temp,{recursive:true,force:true});}
});
