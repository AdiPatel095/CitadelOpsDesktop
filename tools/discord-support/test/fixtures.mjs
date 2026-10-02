import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Store } from '../src/store.mjs';
import { Tickets } from '../src/tickets.mjs';
import { SafeError } from '../src/errors.mjs';
export const config={guildId:'100000000000000001',botId:'100000000000000002',supportRoleId:'100000000000000003',appCategoryId:'100000000000000004'};
export const requester='200000000000000001',staff='200000000000000002',otherStaff='200000000000000003';
export function fixture(t) {
  process.umask(0o077);
  const dir=mkdtempSync(join(tmpdir(),'cit99-test-')); const store=new Store(dir); const adapter=new FakeAdapter(); const tickets=new Tickets(store,adapter,config);
  t.after(() => { try { store.close(); } catch {} rmSync(dir,{recursive:true,force:true}); });
  const input={guildId:config.guildId,requesterId:requester,customId:tickets.modalId('300000000000000001',requester),subject:'Test subject',description:'Test description'};
  return {dir,store,adapter,tickets,input,context:ticket => ({guildId:config.guildId,channelId:ticket.channel_id,actorId:staff})};
}
export class FakeAdapter {
  channels=new Map(); messages=new Map(); authorized=new Set([staff,otherStaff]); counts={create:0,send:0,readOnly:0}; failures={};
  fail(step) { if (this.failures[step]) { this.failures[step]--; throw new SafeError('SIMULATED_NETWORK_FAILURE',false); } }
  async findTicket(t) { this.fail('find'); return this.channels.get(t.id); }
  async createTicket(t) { this.counts.create++; this.fail('beforeCreate'); const c={id:`4000000000000000${String(this.counts.create).padStart(2,'0')}`,readonly:false}; this.channels.set(t.id,c); this.fail('afterCreate'); return c; }
  async validateTicket(t) { if (!this.channels.has(t.id) || this.channels.get(t.id).id !== t.channel_id) throw new SafeError('TICKET_CHANNEL_MISMATCH'); }
  async findInitial(t) { return this.messages.get(t.id); }
  async sendInitial(t) { this.counts.send++; const m={id:`5000000000000000${String(this.counts.send).padStart(2,'0')}`}; this.messages.set(t.id,m); this.fail('afterSend'); return m; }
  async validateControl(t) { if (this.messages.get(t.id)?.id !== t.message_id) throw new SafeError('CONTROL_MESSAGE_MISMATCH'); }
  async isStaff(id) { return this.authorized.has(id); }
  async makeReadOnly(t) { this.counts.readOnly++; this.fail('beforeClose'); this.channels.get(t.id).readonly=true; this.fail('afterClose'); }
  async updateControls(t) { this.fail('update'); this.messages.get(t.id).controls=t.state; }
}
