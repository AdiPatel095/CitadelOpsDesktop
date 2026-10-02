import { existsSync, lstatSync, readdirSync, readFileSync, writeFileSync, mkdirSync, renameSync, symlinkSync, readlinkSync, realpathSync, unlinkSync } from 'node:fs';
import { join, dirname, resolve, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import { isDirectEntry } from '../src/entrypoint.mjs';
import { homedir } from 'node:os';
import { createHash, randomUUID } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { basePath, EXPECTED, validateConfig } from '../src/config.mjs';
import { privateDirectory, privateRead, privateFile, atomicPrivateJSON } from '../src/private.mjs';
import { freshReceipt } from '../src/observability.mjs';
import { SafeError, safeCode, fail } from '../src/errors.mjs';
export const LABEL='com.citadelops.discord-support';
const modulePath=fileURLToPath(new URL('../',import.meta.url));
const escapeXML = value => value.replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;').replaceAll('"','&quot;').replaceAll("'",'&apos;');
export function layout(home = homedir()) {
  const base=basePath(home);
  return {base,service:join(base,'service'),releases:join(base,'releases'),config:join(base,'config.json'),receipt:join(base,'state/readiness.json'),plist:join(home,'Library/LaunchAgents',`${LABEL}.plist`)};
}
export function plistXML(paths,node) {
  return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>${LABEL}</string>
<key>ProgramArguments</key><array><string>${escapeXML(node)}</string><string>${escapeXML(join(paths.service,'src/index.mjs'))}</string></array>
<key>WorkingDirectory</key><string>${escapeXML(paths.service)}</string>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
<key>ThrottleInterval</key><integer>30</integer>
<key>ExitTimeOut</key><integer>35</integer>
<key>Umask</key><integer>63</integer>
<key>StandardOutPath</key><string>/dev/null</string>
<key>StandardErrorPath</key><string>/dev/null</string>
</dict></plist>\n`;
}
export function sourceFiles(source) {
  const files=['package.json','package-lock.json','README.md','.gitignore'];
  function collect(directory) {
    for (const entry of readdirSync(join(source,directory),{withFileTypes:true})) {
      const name=join(directory,entry.name);
      if (entry.isSymbolicLink()) fail('SOURCE_SYMLINK_REJECTED');
      if (entry.isDirectory()) collect(name);
      else if (entry.isFile() && entry.name.endsWith('.mjs')) files.push(name);
      else fail('SOURCE_FILE_UNEXPECTED');
    }
  }
  for (const directory of ['src','scripts','test']) collect(directory);
  for (const file of files) if (!lstatSync(join(source,file)).isFile() || lstatSync(join(source,file)).isSymbolicLink()) fail('SOURCE_FILE_INVALID');
  return files.sort();
}
export function packageInfo(source) {
  const files=sourceFiles(source),hash=createHash('sha256');
  for (const file of files) { hash.update(file+'\0'); hash.update(readFileSync(join(source,file))); hash.update('\0'); }
  return {digest:hash.digest('hex'),files};
}
function run(command,args,cwd) {
  const result=spawnSync(command,args,{cwd,encoding:'utf8',maxBuffer:1024*1024});
  // Child output can contain paths/config; return only codes to users.
  return {ok:result.status === 0,status:result.status,stdout:result.stdout ?? ''};
}
export async function host(action,{home=homedir(),source=modulePath,node=realpathSync(process.execPath),dryRun=false,runner=run} = {}) {
  const paths=layout(home); const domain=`gui/${process.getuid()}`;
  if (!['install','status','stop','uninstall'].includes(action)) fail('HOST_ACTION_INVALID');
  if (action === 'install') {
    if (!isAbsolute(node) || !lstatSync(node).isFile()) fail('NODE_EXECUTABLE_INVALID');
    const info=packageInfo(source);
    const xml=plistXML(paths,node);
    if (dryRun) return {action,dryRun:true,sourceDigest:info.digest,files:info.files,node,service:paths.service,plist:paths.plist,xml};
    process.umask(0o077); privateDirectory(paths.base); privateDirectory(paths.releases);
    // Serialize installers independently of runtime ownership. This lock is held
    // while npm prepares source and across launchd promotion/rollback.
    const { acquireLock } = await import('../src/lock.mjs');
    const release=await acquireLock(join(paths.base,'installer.lock'));
    try {
      if (existsSync(paths.config)) validateConfig(JSON.parse(privateRead(paths.config)),home);
      else atomicPrivateJSON(paths.config,EXPECTED);
      const current=existsSync(paths.service) ? lstatSync(paths.service) : null;
      if (current && !current.isSymbolicLink()) fail('SERVICE_PATH_UNEXPECTED');
      const oldTarget=current ? readlinkSync(paths.service) : null;
      if (oldTarget && (!resolve(dirname(paths.service),oldTarget).startsWith(paths.releases+'/'))) fail('SERVICE_TARGET_UNEXPECTED');
      if (oldTarget && packageInfo(resolve(dirname(paths.service),oldTarget)).digest === info.digest && runner('/bin/launchctl',['print',`${domain}/${LABEL}`]).ok) {
        return {action,sourceDigest:info.digest,service:paths.service,idempotent:true,status:'already registered; verify fresh Gateway readiness separately'};
      }
      const stage=join(paths.releases,`${info.digest}-${randomUUID()}`); privateDirectory(stage);
      for (const file of info.files) {
        mkdirSync(dirname(join(stage,file)),{recursive:true,mode:0o700});
        writeFileSync(join(stage,file),readFileSync(join(source,file)),{mode:0o600,flag:'wx'});
      }
      if (packageInfo(stage).digest !== info.digest) fail('SOURCE_CHANGED_DURING_COPY');
      const which=runner('/usr/bin/which',['npm']);
      if (!which.ok) fail('NPM_NOT_FOUND');
      const npm=realpathSync(which.stdout.trim());
      if (!runner(node,[npm,'ci','--ignore-scripts','--no-fund'],stage).ok || !runner(node,[join(stage,'scripts/check.mjs')],stage).ok) fail('PACKAGE_PREPARATION_FAILED');
      const revision=runner('/usr/bin/git',['-C',source,'rev-parse','HEAD']);
      atomicPrivateJSON(join(stage,'source-pin.json'),{digest:info.digest,revision:revision.ok && /^[a-f0-9]{40}\s*$/.test(revision.stdout) ? revision.stdout.trim() : null});
      // Never read the token. Preserve the previous source, plist and all state.
      mkdirSync(dirname(paths.plist),{recursive:true});
      let oldPlist=null;
      if (existsSync(paths.plist)) { privateFile(paths.plist); oldPlist=readFileSync(paths.plist); }
      const loaded=runner('/bin/launchctl',['print',`${domain}/${LABEL}`]).ok;
      if (loaded && !runner('/bin/launchctl',['bootout',`${domain}/${LABEL}`]).ok) fail('SERVICE_STOP_FAILED');
      const promote = target => {
        const temporary=join(paths.base,`service-${randomUUID()}.tmp`);
        symlinkSync(target,temporary); renameSync(temporary,paths.service);
      };
      try {
        promote(stage);
        const tmp=paths.plist+'.'+randomUUID()+'.tmp'; writeFileSync(tmp,xml,{mode:0o600,flag:'wx'}); renameSync(tmp,paths.plist);
        if (!runner('/bin/launchctl',['bootstrap',domain,paths.plist]).ok) fail('SERVICE_START_FAILED');
        atomicPrivateJSON(join(paths.base,'installation.json'),{digest:info.digest,previous:oldTarget,current:stage,installedAt:new Date().toISOString()});
      } catch(error) {
        runner('/bin/launchctl',['bootout',`${domain}/${LABEL}`]);
        if (oldTarget) promote(oldTarget); else if (existsSync(paths.service)) unlinkSync(paths.service);
        if (oldPlist) writeFileSync(paths.plist,oldPlist,{mode:0o600}); else if (existsSync(paths.plist)) unlinkSync(paths.plist);
        if (loaded && oldPlist && !runner('/bin/launchctl',['bootstrap',domain,paths.plist]).ok) throw new SafeError('ROLLBACK_START_FAILED');
        throw error;
      }
      return {action,sourceDigest:info.digest,service:paths.service,status:'registered; verify fresh Gateway readiness separately'};
    } finally { await release(); }
  }
  if (dryRun) return {action,dryRun:true,target:`${domain}/${LABEL}`,statePreserved:true};
  const launched=runner('/bin/launchctl',['print',`${domain}/${LABEL}`]);
  if (action === 'status') {
    const pid=Number(/^\s*pid = (\d+)$/m.exec(launched.stdout)?.[1]) || null;
    let alive=false,receipt;
    if (pid) { try { process.kill(pid,0); alive=true; } catch {} }
    try { receipt=JSON.parse(privateRead(paths.receipt)); } catch { /* missing/stale is unready */ }
    return {registered:launched.ok,pid,alive,gatewayReady:launched.ok && alive && freshReceipt(receipt,EXPECTED,pid)};
  }
  if (launched.ok && !runner('/bin/launchctl',['bootout',`${domain}/${LABEL}`]).ok) fail('SERVICE_STOP_FAILED');
  if (action === 'uninstall' && existsSync(paths.plist)) {
    privateFile(paths.plist); unlinkSync(paths.plist);
  }
  return {action,status:'unregistered',statePreserved:true};
}
if (isDirectEntry(import.meta.url)) {
  const args=process.argv.slice(2);
  if (args.some(a => !['install','status','stop','uninstall','--dry-run'].includes(a)) || args.filter(a => a !== '--dry-run').length !== 1) { process.stderr.write('Usage: host.mjs install|status|stop|uninstall [--dry-run]\n'); process.exitCode=1; }
  else host(args.find(a => a !== '--dry-run'),{dryRun:args.includes('--dry-run')}).then(result => process.stdout.write(JSON.stringify(result,null,2)+'\n')).catch(error => { process.stderr.write(safeCode(error)+'\n'); process.exitCode=1; });
}
