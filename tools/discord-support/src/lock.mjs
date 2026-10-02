import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { privateFile } from './private.mjs';
import { SafeError } from './errors.mjs';
export async function acquireLock(path,onLost = () => {}) {
  privateFile(path);
  const child = spawn('/usr/bin/lockf',['-k','-s','-t','0',path,process.execPath,fileURLToPath(new URL('./lock-holder.mjs',import.meta.url))],{stdio:['pipe','pipe','ignore']});
  let held = false, releasing = false;
  const closed = new Promise(resolve => child.once('close',resolve));
  await new Promise((resolve,reject) => {
    let output = '';
    const timeout = setTimeout(() => { child.stdin.end(); reject(new SafeError('LOCK_TIMEOUT')); },5000);
    child.stdout.on('data',buffer => {
      output += buffer.toString();
      if (output === 'LOCKED\n') { held=true; clearTimeout(timeout); resolve(); }
    });
    child.once('error',() => { clearTimeout(timeout); reject(new SafeError('LOCK_UNAVAILABLE')); });
    child.once('close',() => {
      clearTimeout(timeout);
      if (!held) reject(new SafeError('STORE_ALREADY_OWNED'));
      else if (!releasing) onLost();
    });
  });
  return async () => { releasing=true; child.stdin.end(); await closed; };
}
