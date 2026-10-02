import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { Events } from 'discord.js';
import { basePath, readConfig, readToken } from './config.mjs';
import { privateDirectory } from './private.mjs';
import { acquireLock } from './lock.mjs';
import { Store } from './store.mjs';
import { Tickets, Serial } from './tickets.mjs';
import { createClient, DiscordAdapter, safeAdapter, handleInteraction } from './discord.mjs';
import { Readiness, logger } from './observability.mjs';
import { discordError, SafeError } from './errors.mjs';
export async function main() {
  process.umask(0o077);
  const base = basePath(); privateDirectory(base);
  const log = logger(join(base,'logs'));
  const config = readConfig(join(base,'config.json'));
  const state = join(base,'state'); privateDirectory(state);
  let shutdown, stopping=false, store, tickets, client, receipt, release;
  const recovery = new Serial(); let heartbeat, retry, failures=0;
  async function stop(code = 0) {
    if (shutdown) return shutdown;
    stopping=true; tickets && (tickets.accepting=false);
    clearInterval(heartbeat); clearTimeout(retry);
    receipt?.clear(code ? 'fatal' : 'stopping'); log(code ? 'fatal' : 'stopping');
    shutdown = (async () => {
      const deadline = setTimeout(() => process.exit(code || 1),30_000);
      // Stop taking gateway actions, then allow durable in-flight work to settle.
      await client?.destroy();
      await recovery.drain();
      await tickets?.shutdown();
      store?.close();
      receipt?.clear(code ? 'fatal' : 'stopped'); log('stopped');
      await release?.(); clearTimeout(deadline);
      process.exitCode=code;
    })();
    return shutdown;
  }
  const fault = error => {
    const e = discordError(error);
    log(e.fatal ? 'fatal' : 'retry',e);
    if (e.fatal) { void stop(1); return; }
    tickets && (tickets.accepting=false); receipt?.clear('retry');
    schedule();
  };
  function schedule() {
    if (stopping || retry) return;
    retry=setTimeout(() => { retry=null; if (client?.isReady()) void recover(); },Math.min(30_000,1000*2**Math.min(failures++,5)));
  }
  function offline() {
    if (stopping) return;
    tickets.accepting=false; receipt.clear(); log('offline');
  }
  function recover() {
    return recovery.run('gateway',async () => {
      if (stopping || !client.isReady()) return;
      tickets.accepting=false;
      receipt.clear('reconciling'); const generation=receipt.generation;
      try {
        await adapter.bootstrap(); await tickets.reconcile();
        if (!stopping && client.isReady() && receipt.markReady(generation)) { tickets.accepting=true; failures=0; log('ready'); }
      } catch(e) { fault(e); }
    });
  }
  let adapter;
  try {
    release = await acquireLock(join(state,'owner.lock'),() => { log('fatal',new SafeError('STATE_LOCK_LOST')); process.exit(1); });
    receipt = new Readiness(join(state,'readiness.json'),config); receipt.write(); log('starting');
    store = new Store(state); client=createClient(); adapter=safeAdapter(new DiscordAdapter(client,store,config));
    tickets=new Tickets(store,adapter,config); tickets.accepting=false;
    const ready = () => !stopping && receipt.status === 'ready' && client.isReady();
    client.on(Events.InteractionCreate,i => { void handleInteraction(i,tickets,adapter,ready,e => {
      log('action-failed',e);
      // User mistakes/denied actions must not take down the service. Infrastructure
      // failures trigger reconciliation; privacy/identity faults remain fatal.
      if (['DISCORD_RETRY_REQUIRED'].includes(e.code) || /MISMATCH|UNCERTAIN|DUPLICATE|MISSING|UNAVAILABLE|ADMIN_REQUIRED/.test(e.code)) fault(e);
    }); });
    client.on(Events.ClientReady,() => { void recover(); });
    client.on(Events.ShardDisconnect,offline); client.on(Events.ShardReconnecting,offline);
    client.on(Events.ShardResume,() => { void recover(); });
    client.on(Events.ShardReady,() => { if (client.isReady()) void recover(); });
    client.on(Events.Error,fault); client.on(Events.ShardError,fault);
    client.on(Events.Warn,() => log('retry',new SafeError('GATEWAY_WARNING',false)));
    client.on(Events.Invalidated,() => fault(new SafeError('GATEWAY_INVALIDATED')));
    process.once('SIGTERM',() => { void stop(); }); process.once('SIGINT',() => { void stop(); });
    process.on('uncaughtException',() => fault(new SafeError('UNCAUGHT_FAILURE')));
    process.on('unhandledRejection',() => fault(new SafeError('UNHANDLED_FAILURE')));
    heartbeat=setInterval(() => {
      if (!client.isReady()) { if (receipt.status !== 'offline') offline(); }
      else if (receipt.status === 'ready' && Date.now()-receipt.validatedAt >= 60_000) void recover();
      receipt.write();
    },15_000);
    await client.login(readToken(config));
  } catch(error) { log('fatal',discordError(error)); await stop(1); }
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(() => { process.stderr.write('SERVICE_START_FAILED\n'); process.exitCode=1; });
}
