import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync,realpathSync,rmSync,mkdirSync,writeFileSync,readFileSync,symlinkSync,existsSync,readlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join,dirname } from 'node:path';
import { fileURLToPath,pathToFileURL } from 'node:url';
import { spawnSync } from 'node:child_process';
import { packageInfo,layout,plistXML } from '../scripts/host.mjs';
import { isDirectEntry } from '../src/entrypoint.mjs';
const source=fileURLToPath(new URL('../',import.meta.url));
function deployedFixture(t) {
  process.umask(0o077);
  // Canonicalize /tmp itself to isolate the installed service symlink. Spaces,
  // '#' and '%' also exercise URL decoding rather than raw URL/path comparison.
  const fixtureHome=realpathSync(mkdtempSync(join(tmpdir(),'cit99 entry #%-')));
  t.after(() => rmSync(fixtureHome,{recursive:true,force:true}));
  const paths=layout(fixtureHome),info=packageInfo(source);
  const release=join(paths.releases,info.digest+'-fixture');
  mkdirSync(release,{recursive:true,mode:0o700});
  for (const file of info.files) {
    mkdirSync(dirname(join(release,file)),{recursive:true,mode:0o700});
    writeFileSync(join(release,file),readFileSync(join(source,file)),{mode:0o600});
  }
  // Dependencies are unchanged and read from the verified isolated module;
  // no npm install or live source/config/credential directory is accessed.
  symlinkSync(join(source,'node_modules'),join(release,'node_modules'));
  symlinkSync(release,paths.service);
  // Intentionally invalid private config guarantees failure before token/store,
  // Discord client creation or any networking, even when main actually runs.
  writeFileSync(paths.config,'{}',{mode:0o600});
  const xml=plistXML(paths,realpathSync(process.execPath));
  const section=/<key>ProgramArguments<\/key><array>(.*?)<\/array>/s.exec(xml)[1];
  const argv=[...section.matchAll(/<string>(.*?)<\/string>/g)].map(m => m[1]);
  const run = (args,cwd=fixtureHome) => spawnSync(argv[0],args,{cwd,
    env:{...process.env,HOME:fixtureHome,NODE_OPTIONS:''},encoding:'utf8',timeout:10_000,maxBuffer:64*1024});
  return {fixtureHome,paths,release,info,argv,run};
}
for (const entry of ['canonical','stable symlink']) test(`actual service ${entry} entry runs startup against only invalid fixture config`,t => {
  const f=deployedFixture(t);
  const args=entry === 'canonical' ? [join(f.release,'src/index.mjs')] : f.argv.slice(1);
  assert.equal(readlinkSync(f.paths.service),f.release);
  const result=f.run(args);
  assert.equal(result.error,undefined);
  assert.equal(result.status,1,'startup must execute and reject fixture config, rather than silently exit zero');
  assert.equal(result.stdout,''); assert.equal(result.stderr,'SERVICE_START_FAILED\n');
  assert.ok(existsSync(join(f.paths.base,'logs')),'main reached the isolated private log setup');
  assert.equal(existsSync(join(f.paths.base,'state')),false,'config rejection occurred before state/token/Gateway setup');
});
test('importing actual service and host modules through the deployed symlink leaves their entrypoints inactive',t => {
  const f=deployedFixture(t),driver=join(f.fixtureHome,'import-driver.mjs');
  const imports=['src/index.mjs','scripts/host.mjs'].map(file => `await import(${JSON.stringify(pathToFileURL(join(f.paths.service,file)).href)});`).join('\n');
  writeFileSync(driver,imports+"\nprocess.stdout.write('IMPORTED_ONLY\\n');\n",{mode:0o600});
  const result=f.run([driver]);
  assert.equal(result.error,undefined); assert.equal(result.status,0);
  assert.equal(result.stdout,'IMPORTED_ONLY\n'); assert.equal(result.stderr,'');
  assert.equal(existsSync(join(f.paths.base,'logs')),false); assert.equal(existsSync(join(f.paths.base,'state')),false);
});
for (const entry of ['canonical','stable symlink']) test(`actual host ${entry} CLI emits deployed-shape dry-run packaging output`,t => {
  const f=deployedFixture(t),script=join(entry === 'canonical' ? f.release : f.paths.service,'scripts/host.mjs');
  const result=f.run([script,'install','--dry-run']);
  assert.equal(result.error,undefined); assert.equal(result.status,0); assert.equal(result.stderr,'');
  assert.notEqual(result.stdout.trim(),'','the actual CLI guard must dispatch the dry-run action');
  const receipt=JSON.parse(result.stdout);
  assert.equal(receipt.dryRun,true); assert.equal(receipt.sourceDigest,f.info.digest);
  assert.equal(receipt.service,f.paths.service); assert.equal(receipt.plist,f.paths.plist);
  assert.equal(receipt.xml,plistXML(f.paths,realpathSync(process.execPath)));
  assert.equal(existsSync(f.paths.plist),false); assert.equal(existsSync(join(f.paths.base,'logs')),false);
  assert.equal(existsSync(join(f.paths.base,'state')),false);
});

test('entry identity safely rejects missing argv/files and non-file module URLs',t => {
  const f=deployedFixture(t),script=join(f.release,'src/index.mjs'),url=pathToFileURL(script).href;
  assert.equal(isDirectEntry(url,''),false);
  assert.equal(isDirectEntry(url,join(f.fixtureHome,'missing.mjs')),false);
  assert.equal(isDirectEntry('data:text/javascript,export{}',script),false);
  assert.equal(isDirectEntry('file:///invalid%2fmodule.mjs',script),false);
});
