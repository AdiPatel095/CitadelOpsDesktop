import { appendFileSync, existsSync, statSync, renameSync, unlinkSync } from 'node:fs';
import { join } from 'node:path';
import { privateDirectory, privateFile, atomicPrivateJSON } from './private.mjs';
import { safeCode } from './errors.mjs';
const EVENTS = new Set(['starting','ready','offline','retry','fatal','action-failed','stopping','stopped']);
export function logger(directory,limit = 1024*1024) {
  privateDirectory(directory);
  const path = join(directory,'service.log');
  return (event,error) => {
    if (!EVENTS.has(event)) return;
    privateFile(path);
    if (statSync(path).size >= limit) {
      if (existsSync(`${path}.2`)) unlinkSync(`${path}.2`);
      if (existsSync(`${path}.1`)) renameSync(`${path}.1`,`${path}.2`);
      renameSync(path,`${path}.1`);
      privateFile(path);
    }
    appendFileSync(path,JSON.stringify({at:new Date().toISOString(),event,code:error ? safeCode(error) : undefined})+'\n');
  };
}
export class Readiness {
  constructor(path,config,now = Date.now) { this.path=path; this.config=config; this.now=now; this.generation=0; this.status='starting'; this.validatedAt=null; this.checks=0; }
  clear(status = 'offline') { this.generation++; this.status=status; this.validatedAt=null; this.write(); }
  markReady(generation) {
    if (generation !== this.generation) return false;
    this.status='ready'; this.validatedAt=this.now(); this.checks++; this.write(); return true;
  }
  write() { atomicPrivateJSON(this.path,{pid:process.pid,botId:this.config.botId,guildId:this.config.guildId,status:this.status,heartbeat:this.now(),validatedAt:this.validatedAt,checks:this.checks}); }
}
export function freshReceipt(receipt,config,pid,now = Date.now()) {
  return receipt?.status === 'ready' && receipt.pid === pid && receipt.botId === config.botId && receipt.guildId === config.guildId && Number.isFinite(receipt.heartbeat) && Number.isFinite(receipt.validatedAt) && now >= receipt.heartbeat && now-receipt.heartbeat < 45_000 && now >= receipt.validatedAt && now-receipt.validatedAt < 120_000;
}
