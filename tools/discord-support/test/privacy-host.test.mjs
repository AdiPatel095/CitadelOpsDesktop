import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync,rmSync,writeFileSync,readFileSync,chmodSync,statSync,symlinkSync,existsSync,readlinkSync,mkdirSync,appendFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawn,spawnSync } from 'node:child_process';
import { once } from 'node:events';
import { fileURLToPath } from 'node:url';
import { PermissionFlagsBits as P } from 'discord.js';
import { validateConfig,EXPECTED,readToken } from '../src/config.mjs';
import { privateDirectory,privateRead } from '../src/private.mjs';
import { logger,Readiness,freshReceipt } from '../src/observability.mjs';
import { acquireLock } from '../src/lock.mjs';
import { overwrites,exactOverwrites,currentStaff } from '../src/permissions.mjs';
import { host,layout,packageInfo,plistXML,LABEL } from '../scripts/host.mjs';
import { createClient,MENTIONS,controls } from '../src/discord.mjs';
import { fixture,config,requester,staff } from './fixtures.mjs';
function temp(t) { process.umask(0o077); const p=mkdtempSync(join(tmpdir(),'cit99-private-')); t.after(() => rmSync(p,{recursive:true,force:true})); return p; }
function calculate(list,id,roles,base = P.ViewChannel|P.SendMessages|P.ReadMessageHistory|P.CreatePublicThreads|P.SendMessagesInThreads|P.ManageWebhooks|P.MentionEveryone|P.CreateInstantInvite) {
  if (base&P.Administrator) return ~0n;
  const everyone=list.find(e => e.id === config.guildId); let value=(base&~everyone.deny)|everyone.allow;
  const rs=list.filter(e => e.type === 0 && roles.includes(e.id)); value=(value&~rs.reduce((b,e)=>b|e.deny,0n))|rs.reduce((b,e)=>b|e.allow,0n);
  const m=list.find(e => e.id === id && e.type === 1); return m ? (value&~m.deny)|m.allow : value;
}
test('permission hierarchy isolates other customers/Moderator/Developer; requester thread and management powers denied',() => {
  const list=overwrites(config,'ticket',requester);
  for (const roles of [[],['100000000000000011'],['100000000000000012']]) assert.equal(calculate(list,'200000000000000099',roles)&P.ViewChannel,0n);
  const own=calculate(list,requester,[]); assert.ok(own&P.ViewChannel); assert.ok(own&P.SendMessages); assert.ok(own&P.AttachFiles);
  for (const p of [P.CreatePublicThreads,P.CreatePrivateThreads,P.SendMessagesInThreads,P.CreateInstantInvite,P.MentionEveryone,P.ManageChannels,P.ManageWebhooks]) assert.equal(own&p,0n);
  assert.ok(calculate(list,staff,[config.supportRoleId])&P.SendMessages);
  assert.ok(calculate(list,'200000000000000099',[],P.Administrator)&P.ViewChannel);
  const closed=overwrites(config,'ticket',requester,true);
  // Member-specific deny also wins if requester happens to have Support role.
  for (const roles of [[],[config.supportRoleId]]) {
    const p=calculate(closed,requester,roles); assert.ok(p&P.ViewChannel); assert.ok(p&P.ReadMessageHistory);
    for (const flag of [P.SendMessages,P.SendMessagesInThreads,P.CreatePublicThreads,P.CreatePrivateThreads,P.AttachFiles,P.AddReactions]) assert.equal(p&flag,0n);
  }
  assert.equal(calculate(overwrites(config,'panel'),'200000000000000099',[])&P.SendMessages,0n);
});
test('privacy validator rejects extra member/role grants and stale staff roles are not sufficient',() => {
  const expected=overwrites(config,'ticket',requester);
  const actual=expected.map(e => ({...e,allow:{bitfield:e.allow},deny:{bitfield:e.deny}}));
  const channel={permissionOverwrites:{cache:new Map(actual.map(e => [e.id,e]))}};
  exactOverwrites(channel,expected); channel.permissionOverwrites.cache.set('200000000000000099',{id:'200000000000000099',type:1,allow:{bitfield:P.ViewChannel},deny:{bitfield:0n}});
  assert.throws(() => exactOverwrites(channel,expected),{code:'PRIVACY_OVERWRITE_MISMATCH'});
  const member={guild:{id:config.guildId},permissions:{has:() => false},roles:{cache:new Set([config.supportRoleId])}};
  assert.equal(currentStaff(member,config),true); member.roles.cache.clear(); assert.equal(currentStaff(member,config),false);
});
test('configuration is fixed, token fixtures must be owned private nonsymlink regular files',t => {
  const home=temp(t); assert.deepEqual(validateConfig(EXPECTED,home),{...EXPECTED,tokenPath:join(home,'.config/citadel-ops-discord/bot-token')});
  assert.throws(() => validateConfig({...EXPECTED,guildId:config.guildId}),{code:'CONFIG_IDENTITY_MISMATCH'});
  assert.throws(() => validateConfig({...EXPECTED,unexpected:true}),{code:'CONFIG_UNKNOWN_KEY'});
  assert.throws(() => validateConfig({...EXPECTED,tokenPath:'relative'}),{code:'CONFIG_TOKEN_PATH_INVALID'});
  const tokenPath=join(home,'fake-token'); writeFileSync(tokenPath,'fixture-only-value\n',{mode:0o600});
  assert.equal(readToken({tokenPath}),'fixture-only-value'); chmodSync(tokenPath,0o644); assert.throws(() => readToken({tokenPath}),{code:'TOKEN_PRIVATE_FILE_REQUIRED'});
  const fifo=join(home,'fifo'); assert.equal(spawnSync('/usr/bin/mkfifo',[fifo]).status,0); assert.throws(() => readToken({tokenPath:fifo}),{code:'TOKEN_PRIVATE_FILE_REQUIRED'});
  chmodSync(tokenPath,0o600); const link=join(home,'link'); symlinkSync(tokenPath,link); assert.throws(() => readToken({tokenPath:link}));
  writeFileSync(tokenPath,'  '); assert.throws(() => readToken({tokenPath}),{code:'TOKEN_INVALID'});
  chmodSync(home,0o755); assert.throws(() => privateDirectory(home),{code:'PRIVATE_DIRECTORY_REQUIRED'}); chmodSync(home,0o700);
});
test('state files remain 0600, logs ignore raw exceptions and rotate boundedly',t => {
  const f=fixture(t); assert.equal(statSync(join(f.dir,'tickets.sqlite')).mode&0o777,0o600);
  const dir=temp(t),log=logger(dir,200); const secret=new Error('TOKEN_should_never_appear @everyone subject body');
  for (let i=0;i<50;i++) log('retry',secret);
  for (const name of ['service.log','service.log.1','service.log.2']) {
    const path=join(dir,name); assert.ok(statSync(path).size < 400); assert.equal(statSync(path).mode&0o777,0o600);
    const contents=readFileSync(path,'utf8'); assert.ok(contents.includes('OPERATION_FAILED')); assert.ok(!contents.includes('TOKEN_should'));
  }
});
test('readiness cannot win after disconnect and status rejects stale/wrong PID/identity receipts',t => {
  const dir=temp(t); let now=1_000_000; const r=new Readiness(join(dir,'ready.json'),config,() => now);
  const generation=r.generation; r.clear(); assert.equal(r.markReady(generation),false); r.markReady(r.generation);
  const receipt=JSON.parse(privateRead(r.path)); assert.equal(freshReceipt(receipt,config,process.pid,now),true);
  assert.equal(freshReceipt(receipt,config,process.pid+1,now),false); assert.equal(freshReceipt(receipt,{...config,botId:'bad'},process.pid,now),false);
  now+=45_001; assert.equal(freshReceipt(receipt,config,process.pid,now),false); r.clear(); assert.equal(freshReceipt(JSON.parse(privateRead(r.path)),config,process.pid,now),false);
});
test('kernel lock prevents a second owner, preserves inode, and recovers after release',async t => {
  const dir=temp(t),path=join(dir,'owner.lock'),release=await acquireLock(path); const ino=statSync(path).ino;
  try { await assert.rejects(acquireLock(path),{code:'STORE_ALREADY_OWNED'}); } finally { await release(); }
  const next=await acquireLock(path); assert.equal(statSync(path).ino,ino); await next();
});
test('kernel lock recovers after the owning process is killed',async t => {
  const dir=temp(t),path=join(dir,'owner.lock'),module=new URL('../src/lock.mjs',import.meta.url).href;
  const child=spawn(process.execPath,['--input-type=module','-e',`import {acquireLock} from ${JSON.stringify(module)}; await acquireLock(${JSON.stringify(path)}); process.stdout.write('owned\\n');`],{stdio:['ignore','pipe','ignore']});
  await once(child.stdout,'data'); const exit=once(child,'exit'); child.kill('SIGKILL'); await exit;
  let release;
  for (let n=0;n<40;n++) { try { release=await acquireLock(path); break; } catch { await new Promise(r => setTimeout(r,25)); } }
  assert.ok(release); await release();
});
test('client has Guilds only, disabled ambiguous transport retry, suppressed mentions and disabled closed controls',async () => {
  const client=createClient(); assert.equal(client.options.intents.bitfield,1); assert.equal(client.rest.options.retries,0); assert.deepEqual(client.options.allowedMentions,MENTIONS);
  assert.equal(client.listeners('messageCreate').length,0); await client.destroy();
  const buttons=controls({id:'00000000-0000-4000-8000-000000000001',state:'closed'})[0].toJSON().components;
  assert.ok(buttons.every(b => b.disabled));
});
test('installer dry-run is read-only and packages only isolated source; plist has absolute pinned paths and no secrets',async t => {
  const home=temp(t),result=await host('install',{home,dryRun:true});
  assert.equal(existsSync(layout(home).base),false); assert.ok(result.files.every(f => !f.includes('node_modules') && !f.includes('config.json')));
  assert.ok(result.xml.includes('<integer>30</integer>')); assert.ok(result.xml.includes(LABEL)); assert.ok(!result.xml.includes('EnvironmentVariables'));
  assert.ok(!result.xml.includes('bot-token')); assert.ok(result.xml.includes(result.node));
  assert.equal(result.sourceDigest,packageInfo(fileURLToPath(new URL('../',import.meta.url))).digest);
  const xml=plistXML({...layout(home),service:'/private/test & <path>'},'/private/node'); assert.ok(xml.includes('&amp;')); assert.ok(xml.includes('&lt;'));
});
test('installer fixture stages source, preserves state and old source on preparation/registration failure',async t => {
  const home=temp(t),paths=layout(home),source=join(home,'source');
  const original=fileURLToPath(new URL('../',import.meta.url));
  for (const file of packageInfo(original).files) { mkdirSync(join(source,file,'..'),{recursive:true}); writeFileSync(join(source,file),readFileSync(join(original,file)),{mode:0o600}); }
  let failAt=null,loaded=false; const commands=[];
  const runner=(command,args) => {
    commands.push([command,args]);
    if (command === '/usr/bin/which') return {ok:true,stdout:'/opt/homebrew/bin/npm\n'};
    if (command === '/usr/bin/git') return {ok:true,stdout:'a'.repeat(40)};
    if (command === '/bin/launchctl') {
      const action=args[0];
      if (action === 'print') return {ok:loaded,stdout:loaded ? `pid = ${process.pid}\n` : ''};
      if (action === 'bootstrap') { if (failAt === 'bootstrap') { failAt=null; return {ok:false,stdout:''}; } loaded=true; }
      if (action === 'bootout') loaded=false;
      return {ok:true,stdout:''};
    }
    if (failAt === 'ci') return {ok:false,stdout:''};
    return {ok:true,stdout:''};
  };
  await host('install',{home,source,runner});
  assert.equal((await host('install',{home,source,runner})).idempotent,true);
  const old=readlinkSync(paths.service),oldXML=readFileSync(paths.plist,'utf8');
  mkdirSync(join(paths.base,'state'),{mode:0o700}); writeFileSync(join(paths.base,'state/retained-fixture'),'retained',{mode:0o600});
  appendFileSync(join(source,'README.md'),'\nFixture update\n');
  failAt='ci'; await assert.rejects(host('install',{home,source,runner}),{code:'PACKAGE_PREPARATION_FAILED'}); assert.equal(readlinkSync(paths.service),old); assert.equal(loaded,true);
  failAt='bootstrap'; await assert.rejects(host('install',{home,source,runner}),{code:'SERVICE_START_FAILED'}); assert.equal(readlinkSync(paths.service),old); assert.equal(readFileSync(paths.plist,'utf8'),oldXML); assert.equal(loaded,true);
  const status=await host('status',{home,runner}); assert.equal(status.registered,true); assert.equal(status.gatewayReady,false);
  await host('stop',{home,runner}); assert.equal(loaded,false); assert.equal(existsSync(paths.plist),true);
  await host('uninstall',{home,runner}); assert.equal(existsSync(paths.plist),false); assert.equal(readFileSync(join(paths.base,'state/retained-fixture'),'utf8'),'retained');
  assert.ok(commands.filter(([c]) => c === '/bin/launchctl').every(([,args]) => args.some(a => a.includes(LABEL))));
});
