import { createHash, createHmac, timingSafeEqual } from 'node:crypto';
import { SafeError, fail } from './errors.mjs';
export const SNOWFLAKE = /^\d{17,20}$/;
export const TICKET_ID = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
export function intake(subject, description) {
  if (typeof subject !== 'string' || typeof description !== 'string' || !subject.trim() || !description.trim() || subject.length > 100 || description.length > 1500 || /[\u0000-\u0008\u000b\u000c\u000e-\u001f]/.test(subject + description)) fail('INPUT_INVALID');
  return { subject: subject.trim(), description: description.trim() };
}
// In-memory keyed queues serialize submit/claim/close/recovery, including different requests.
export class Serial {
  #pending = new Map();
  run(key, fn) {
    const result = (this.#pending.get(key) ?? Promise.resolve()).catch(() => {}).then(fn);
    this.#pending.set(key, result);
    return result.finally(() => { if (this.#pending.get(key) === result) this.#pending.delete(key); });
  }
  async drain() { await Promise.allSettled([...this.#pending.values()]); }
}
export class Tickets {
  constructor(store, adapter, config) {
    this.store = store; this.adapter = adapter; this.config = config; this.serial = new Serial();
    this.secret = store.signingKey(); this.accepting = true;
  }
  modalId(buttonId, requesterId) {
    if (!SNOWFLAKE.test(buttonId) || !SNOWFLAKE.test(requesterId)) fail('INTERACTION_INVALID');
    return `ticket:${buttonId}:${this.#sign(`${buttonId}:${requesterId}`)}`;
  }
  #sign(text) { return createHmac('sha256', this.secret).update(`${this.config.guildId}:${text}`).digest('hex').slice(0,24); }
  verifyModal(customId, requesterId) {
    const match = /^ticket:(\d{17,20}):([a-f0-9]{24})$/.exec(customId);
    if (!match || !SNOWFLAKE.test(requesterId) || !timingSafeEqual(Buffer.from(match[2]), Buffer.from(this.#sign(`${match[1]}:${requesterId}`)))) fail('MODAL_INVALID');
    return match[1];
  }
  async submit({ guildId, requesterId, customId, subject, description }) {
    if (!this.accepting) fail('SERVICE_NOT_READY');
    if (guildId !== this.config.guildId) fail('GUILD_MISMATCH');
    const buttonId = this.verifyModal(customId, requesterId);
    const input = intake(subject, description);
    const requestKey = createHash('sha256').update(`${guildId}:${requesterId}:${buttonId}`).digest('hex');
    return this.serial.run(requestKey, async () => {
      const t = this.store.reserve({requestKey,guildId,requesterId,...input});
      return this.serial.run(t.id, () => this.#recover(t.id));
    });
  }
  async #recover(id) {
    let t = this.store.ticket(id);
    if (t.state === 'creating') {
      if (!t.channel_id) {
        let channel = await this.adapter.findTicket(t);
        if (!channel) {
          if (t.create_attempted) throw new SafeError('CHANNEL_CREATION_UNCERTAIN');
          this.store.update(id, {create_attempted:1});
          channel = await this.adapter.createTicket(t);
        }
        t = this.store.update(id, {channel_id:channel.id});
      }
      await this.adapter.validateTicket(t);
      if (!t.message_id) {
        let message = await this.adapter.findInitial(t);
        if (!message) {
          if (t.message_attempted) throw new SafeError('MESSAGE_CREATION_UNCERTAIN');
          this.store.update(id, {message_attempted:1});
          message = await this.adapter.sendInitial(t);
        }
        t = this.store.update(id, {message_id:message.id});
      }
      await this.adapter.validateControl(t);
      t = this.store.update(id, {state:'open',subject:null,description:null});
      this.store.audit(id,'opened');
    } else {
      await this.adapter.validateTicket(t);
      await this.adapter.validateControl(t);
    }
    if (t.state === 'closing') {
      await this.adapter.makeReadOnly(t);
      t = this.store.update(id, {state:'closed'});
      this.store.audit(id,'closed');
    }
    await this.adapter.updateControls(t);
    return t;
  }
  async #authorized(id, context, states) {
    if (!this.accepting) fail('SERVICE_NOT_READY');
    if (!TICKET_ID.test(id)) fail('CONTROL_INVALID');
    const t = this.store.ticket(id);
    if (context.guildId !== t.guild_id || t.guild_id !== this.config.guildId || context.channelId !== t.channel_id || !SNOWFLAKE.test(context.actorId) || !states.includes(t.state)) fail('CONTROL_CONTEXT_INVALID');
    await this.adapter.validateTicket(t);
    await this.adapter.validateControl(t);
    if (!await this.adapter.isStaff(context.actorId)) fail('STAFF_REQUIRED');
    return t;
  }
  claim(id, context) {
    return this.serial.run(id, async () => {
      await this.#authorized(id,context,['open']);
      const t = this.store.claim(id,context.actorId);
      this.store.audit(id,'claimed');
      await this.adapter.updateControls(t);
      return t;
    });
  }
  confirmClose(id, context) {
    return this.serial.run(id,async () => {
      await this.#authorized(id,context,['open']);
      // Actor/channel binding, expiry; no interaction tokens persisted.
      const expires = Date.now() + 120_000;
      return `confirm:${id}:${expires}:${this.#sign(`${id}:${context.actorId}:${context.channelId}:${expires}`)}`;
    });
  }
  close(customId, context) {
    const m = /^confirm:([a-f0-9-]{36}):(\d{13}):([a-f0-9]{24})$/.exec(customId);
    if (!m || !TICKET_ID.test(m[1]) || Date.now() > Number(m[2]) || !timingSafeEqual(Buffer.from(m[3]), Buffer.from(this.#sign(`${m[1]}:${context.actorId}:${context.channelId}:${m[2]}`)))) fail('CONFIRMATION_INVALID');
    return this.serial.run(m[1],async () => {
      const t = await this.#authorized(m[1],context,['open','closing','closed']);
      if (t.state === 'open') { this.store.update(t.id,{state:'closing'}); this.store.audit(t.id,'closing'); }
      return this.#recover(t.id);
    });
  }
  async reconcile() { for (const t of this.store.all()) await this.serial.run(t.id, () => this.#recover(t.id)); }
  async shutdown() { this.accepting = false; await this.serial.drain(); }
}
