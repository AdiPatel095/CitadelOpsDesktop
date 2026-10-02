import {
  Client, GatewayIntentBits, Options, ChannelType, PermissionFlagsBits as P,
  ActionRowBuilder, ButtonBuilder, ButtonStyle, ModalBuilder,
  TextInputBuilder, TextInputStyle, MessageFlags, Routes,
} from 'discord.js';
import { createHash } from 'node:crypto';
import { overwrites, exactOverwrites, currentStaff } from './permissions.mjs';
import { fail, discordError, safeCode } from './errors.mjs';
import { TICKET_ID } from './tickets.mjs';
export const MENTIONS = Object.freeze({parse:[],users:[],roles:[],repliedUser:false});
export function createClient() {
  return new Client({intents:[GatewayIntentBits.Guilds],allowedMentions:MENTIONS,
    // Disable transport retries after ambiguous POST acceptance; reconcile our durable intent.
    rest:{retries:0,timeout:15_000},
    makeCache:Options.cacheWithLimits({MessageManager:0,GuildMemberManager:0}),
  });
}
const row = (...buttons) => new ActionRowBuilder().addComponents(buttons);
const button = (id,label,style,disabled = false) => new ButtonBuilder().setCustomId(id).setLabel(label).setStyle(style).setDisabled(disabled);
export function controls(ticket) {
  const disabled = ticket.state === 'closed' || ticket.state === 'closing';
  return [row(button(`claim:${ticket.id}`,ticket.claimed_by ? 'Claimed' : 'Claim',ButtonStyle.Primary,disabled || Boolean(ticket.claimed_by)),button(`close:${ticket.id}`,'Close',ButtonStyle.Danger,disabled))];
}
export class DiscordAdapter {
  constructor(client,store,config) { this.client=client; this.store=store; this.config=config; }
  marker(kind,id = '') { return `citadelops:${this.config.botId}:${kind}${id ? `:${id}` : ''}`; }
  async validateIdentity() {
    if (this.client.user?.id !== this.config.botId || this.client.application?.id !== this.config.botId) fail('BOT_IDENTITY_MISMATCH');
    this.guild = await this.client.guilds.fetch({guild:this.config.guildId,force:true});
    if (this.guild.id !== this.config.guildId) fail('GUILD_MISMATCH');
    // Fetch role definitions from REST before evaluating Administrator permissions.
    // Fresh member role IDs alone must not grant permissions from a stale role cache.
    const roles = await this.guild.roles.fetch();
    const bot = await this.guild.members.fetch({user:this.config.botId,force:true,cache:false});
    if (!bot.user.bot || !bot.permissions.has(P.Administrator)) fail('BOT_ADMIN_REQUIRED');
    const support = roles.get(this.config.supportRoleId);
    if (!support || support.guild.id !== this.config.guildId || support.managed || support.id === this.config.guildId) fail('SUPPORT_ROLE_INVALID');
    const parent = await this.guild.channels.fetch(this.config.appCategoryId,{force:true,cache:false});
    if (!parent || parent.guildId !== this.config.guildId || parent.type !== ChannelType.GuildCategory) fail('APP_CATEGORY_INVALID');
  }
  async channel(id) {
    const c = await this.guild.channels.fetch(id,{force:true,cache:false});
    if (!c || c.guildId !== this.config.guildId) fail('CHANNEL_MISSING');
    return c;
  }
  async channels() { return [...(await this.guild.channels.fetch()).values()].filter(Boolean); }
  unique(items) { if (items.length > 1) fail('DUPLICATE_RESOURCE_MARKER'); return items[0]; }
  // Categories have no topic field. A bot-authored CHANNEL_CREATE audit reason is
  // the recovery marker; never take ownership of an arbitrary matching name.
  async categoryCandidates() {
    const ids = new Set(); let before;
    for (;;) {
      const query = new URLSearchParams({action_type:'10',user_id:this.config.botId,limit:'100'});
      if (before) query.set('before',before);
      const page = await this.client.rest.get(Routes.guildAuditLog(this.config.guildId),{query});
      for (const e of page.audit_log_entries) if (e.user_id === this.config.botId && e.reason === this.marker('category')) ids.add(e.target_id);
      if (page.audit_log_entries.length < 100) break;
      before = page.audit_log_entries.at(-1).id;
    }
    return [...ids];
  }
  async ensureChannel(key,find,create,validate) {
    const record = this.store.resource(key);
    const candidates = await find();
    const candidate = this.unique(candidates);
    if (record.id && candidate && candidate.id !== record.id) fail('RESOURCE_BINDING_MISMATCH');
    let c;
    if (record.id) c = await this.channel(record.id);
    else if (candidate) c = candidate;
    else {
      if (record.attempted) fail('RESOURCE_CREATION_UNCERTAIN');
      this.store.attemptResource(key);
      c = await create();
    }
    await validate(c);
    if (!record.id) this.store.bindResource(key,c.id);
    return c;
  }
  async bootstrap() {
    await this.validateIdentity(); // before any resource mutation
    const all = await this.channels();
    this.category = await this.ensureChannel('category',async () => {
      const ids = await this.categoryCandidates();
      // A marker whose channel vanished is a fault, not permission to replace it.
      return Promise.all(ids.map(id => this.channel(id)));
    },() => this.guild.channels.create({name:'🎫 Private Tickets',type:ChannelType.GuildCategory,permissionOverwrites:overwrites(this.config,'category'),reason:this.marker('category')}),c => this.validateCategory(c));
    this.panel = await this.ensureChannel('panel',async () => all.filter(c => c.topic === this.marker('panel')),() => this.guild.channels.create({name:'open-a-ticket',type:ChannelType.GuildText,parent:this.config.appCategoryId,topic:this.marker('panel'),permissionOverwrites:overwrites(this.config,'panel'),reason:this.marker('panel')}),c => this.validatePanel(c));
    await this.ensurePanelMessage();
  }
  validateCategory(c) {
    if (c.guildId !== this.config.guildId || c.type !== ChannelType.GuildCategory || c.name !== '🎫 Private Tickets' || c.parentId) fail('CATEGORY_MISMATCH');
    exactOverwrites(c,overwrites(this.config,'category'));
  }
  validatePanel(c) {
    if (c.guildId !== this.config.guildId || c.type !== ChannelType.GuildText || c.name !== 'open-a-ticket' || c.parentId !== this.config.appCategoryId || c.topic !== this.marker('panel')) fail('PANEL_MISMATCH');
    exactOverwrites(c,overwrites(this.config,'panel'));
  }
  async validateResources() {
    await this.validateIdentity();
    this.category = await this.channel(this.store.resource('category').id);
    this.panel = await this.channel(this.store.resource('panel').id);
    this.validateCategory(this.category); this.validatePanel(this.panel);
  }
  hasMarker(message,marker) { return message.author.id === this.config.botId && message.embeds.some(e => e.footer?.text === marker); }
  async findMessage(channel,marker,fullHistory = false) {
    const found = []; let before;
    do {
      // REST recovery inspects only bot marker metadata; no message events,
      // customer content collection, logging, or transcript persistence.
      const page = await channel.messages.fetch({limit:100,before,cache:false});
      found.push(...[...page.values()].filter(m => this.hasMarker(m,marker)));
      if (!fullHistory || page.size < 100) break;
      before = page.last().id;
    } while (before);
    return this.unique(found);
  }
  nonce(marker) { return createHash('sha256').update(marker).digest('hex').slice(0,24); }
  async ensurePanelMessage() {
    const marker = this.marker('panel-message');
    const record = this.store.resource('panel-message');
    let m = await this.findMessage(this.panel,marker);
    if (record.id) {
      if (m && m.id !== record.id) fail('PANEL_MESSAGE_MISMATCH');
      m = await this.panel.messages.fetch({message:record.id,force:true,cache:false});
      if (!this.hasMarker(m,marker)) fail('PANEL_MESSAGE_MISMATCH');
    } else if (!m) {
      if (record.attempted) fail('PANEL_MESSAGE_UNCERTAIN');
      this.store.attemptResource('panel-message');
      m = await this.panel.send({embeds:[{title:'Open a support ticket',description:'Each request opens one private conversation with support. Do not share passwords, login codes, or payment-card details.',footer:{text:marker}}],components:[row(button('open-ticket','Open a ticket',ButtonStyle.Primary))],allowedMentions:MENTIONS,nonce:this.nonce(marker),enforceNonce:true});
    }
    if (!record.id) this.store.bindResource('panel-message',m.id);
  }
  async findTicket(t) {
    await this.validateResources();
    const c = this.unique((await this.channels()).filter(c => c.topic === this.marker('ticket',t.id)));
    if (c) this.validateTicketChannel(c,t);
    return c;
  }
  async createTicket(t) {
    await this.validateResources();
    // Requester must currently belong to this guild. No GuildMembers intent needed for REST.
    await this.guild.members.fetch({user:t.requester_id,force:true,cache:false});
    return this.guild.channels.create({name:`ticket-${t.id.slice(0,8)}`,type:ChannelType.GuildText,parent:this.category.id,topic:this.marker('ticket',t.id),permissionOverwrites:overwrites(this.config,'ticket',t.requester_id),reason:this.marker('ticket',t.id)});
  }
  validateTicketChannel(c,t) {
    if (c.type !== ChannelType.GuildText || c.guildId !== this.config.guildId || c.parentId !== this.category.id || c.topic !== this.marker('ticket',t.id)) fail('TICKET_CHANNEL_MISMATCH');
    if (t.state === 'closing') {
      try { exactOverwrites(c,overwrites(this.config,'ticket',t.requester_id,false)); return; }
      catch { /* interrupted close may already have applied the read-only overwrites */ }
    }
    exactOverwrites(c,overwrites(this.config,'ticket',t.requester_id,t.state === 'closed' || t.state === 'closing'));
  }
  async validateTicket(t) {
    await this.validateResources();
    const matches = (await this.channels()).filter(c => c.topic === this.marker('ticket',t.id));
    const c = this.unique(matches);
    if (!c || c.id !== t.channel_id) fail('TICKET_CHANNEL_MISMATCH');
    this.validateTicketChannel(await this.channel(t.channel_id),t);
  }
  async findInitial(t) { return this.findMessage(await this.channel(t.channel_id),this.marker('initial',t.id),true); }
  async sendInitial(t) {
    const marker = this.marker('initial',t.id);
    return (await this.channel(t.channel_id)).send({embeds:[{title:t.subject,description:t.description,footer:{text:marker}}],components:controls({...t,state:'open'}),allowedMentions:MENTIONS,nonce:this.nonce(marker),enforceNonce:true});
  }
  async control(t) {
    const c = await this.channel(t.channel_id);
    const m = await c.messages.fetch({message:t.message_id,force:true,cache:false});
    if (!m || m.channelId !== t.channel_id || !this.hasMarker(m,this.marker('initial',t.id))) fail('CONTROL_MESSAGE_MISMATCH');
    return m;
  }
  async validateControl(t) { await this.control(t); }
  async isStaff(id) {
    await this.guild.roles.fetch();
    return currentStaff(await this.guild.members.fetch({user:id,force:true,cache:false}),this.config);
  }
  async makeReadOnly(t) {
    const c = await this.channel(t.channel_id);
    // One atomic overwrite replacement; current authorized configuration checked beforehand.
    await c.permissionOverwrites.set(overwrites(this.config,'ticket',t.requester_id,true),this.marker('close',t.id));
    exactOverwrites(await this.channel(t.channel_id),overwrites(this.config,'ticket',t.requester_id,true));
  }
  async updateControls(t) { await (await this.control(t)).edit({components:controls(t),allowedMentions:MENTIONS}); }
}
export function safeAdapter(adapter) {
  return new Proxy(adapter,{get(target,key) {
    const value = target[key];
    if (typeof value !== 'function') return value;
    return async (...args) => { try { return await value.apply(target,args); } catch(e) { throw discordError(e); } };
  }});
}
export async function handleInteraction(i,tickets,adapter,isReady,onFault = () => {}) {
  if (!i.isButton() && !i.isModalSubmit()) return;
  try {
    if (i.isButton() && i.customId === 'open-ticket') {
      if (!isReady() || i.guildId !== tickets.config.guildId || i.channelId !== adapter.store.resource('panel').id || i.message.id !== adapter.store.resource('panel-message').id || i.message.author.id !== tickets.config.botId) fail('PANEL_CONTEXT_INVALID');
      const customId = tickets.modalId(i.id,i.user.id);
      const modal = new ModalBuilder().setCustomId(customId).setTitle('Open a support ticket').addComponents(
        new ActionRowBuilder().addComponents(new TextInputBuilder().setCustomId('subject').setLabel('Subject').setStyle(TextInputStyle.Short).setRequired(true).setMaxLength(100)),
        new ActionRowBuilder().addComponents(new TextInputBuilder().setCustomId('description').setLabel('Description').setStyle(TextInputStyle.Paragraph).setRequired(true).setMaxLength(1500)),
      );
      await i.showModal(modal); return;
    }
    await i.deferReply({flags:MessageFlags.Ephemeral});
    if (!isReady() || i.guildId !== tickets.config.guildId) fail('SERVICE_NOT_READY');
    if (i.isModalSubmit()) {
      const t = await tickets.submit({guildId:i.guildId,requesterId:i.user.id,customId:i.customId,subject:i.fields.getTextInputValue('subject'),description:i.fields.getTextInputValue('description')});
      await i.editReply({content:`Your private ticket is ready: <#${t.channel_id}>`,allowedMentions:MENTIONS}); return;
    }
    const ctx = {guildId:i.guildId,channelId:i.channelId,actorId:i.user.id};
    const m = /^(claim|close):([a-f0-9-]{36})$/.exec(i.customId);
    if (m && TICKET_ID.test(m[2])) {
      const t = tickets.store.ticket(m[2]);
      if (i.message.id !== t.message_id || i.message.author.id !== tickets.config.botId) fail('INTERACTION_SOURCE_INVALID');
      if (m[1] === 'claim') { await tickets.claim(m[2],ctx); await i.editReply({content:'Ticket claimed.',allowedMentions:MENTIONS}); }
      else { const id = await tickets.confirmClose(m[2],ctx); await i.editReply({content:'Close this ticket? The requester will retain read-only access.',components:[row(button(id,'Confirm close',ButtonStyle.Danger))],allowedMentions:MENTIONS}); }
    } else if (i.customId.startsWith('confirm:')) {
      // Confirmation was created as an ephemeral response, bound cryptographically to actor/channel.
      await tickets.close(i.customId,ctx);
      await i.editReply({content:'Ticket closed. The conversation is retained.',components:[],allowedMentions:MENTIONS});
    } else fail('CONTROL_INVALID');
  } catch (error) {
    const safe = discordError(error);
    onFault(safe);
    const content = `Unable to complete this action (${safeCode(safe)}). Support can check the service status.`;
    try { if (i.deferred || i.replied) await i.editReply({content,components:[],allowedMentions:MENTIONS}); else await i.reply({content,flags:MessageFlags.Ephemeral,allowedMentions:MENTIONS}); } catch { /* never log interaction/error bodies */ }
  }
}
