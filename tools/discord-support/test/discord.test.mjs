import { test } from 'node:test';
import assert from 'node:assert/strict';
import { ChannelType,Collection,PermissionFlagsBits as P,MessageFlags } from 'discord.js';
import { DiscordAdapter,handleInteraction,MENTIONS } from '../src/discord.mjs';
import { Tickets } from '../src/tickets.mjs';
import { fixture,config,requester,staff } from './fixtures.mjs';
function world(f) {
  const channels=new Collection(),audits=[],members=new Map(),sent=[]; let sequence=1;
  function makeChannel(data) {
    const id=String(400000000000000000n+BigInt(sequence++)),messages=new Collection();
    const c={...data,id,guildId:config.guildId,parentId:data.parent ?? null,
      permissionOverwrites:{cache:new Collection((data.permissionOverwrites??[]).map(e => [e.id,{...e,allow:{bitfield:e.allow},deny:{bitfield:e.deny}}])),async set(list) { this.cache=new Collection(list.map(e => [e.id,{...e,allow:{bitfield:e.allow},deny:{bitfield:e.deny}}])); }},
      messages:{async fetch(options) {
        if (options.message) { if (!messages.has(options.message)) throw Object.assign(new Error('raw private failure'),{code:10008}); return messages.get(options.message); }
        const values=[...messages.values()].reverse().filter(m => !options.before || BigInt(m.id)<BigInt(options.before)).slice(0,options.limit);
        return new Collection(values.map(m => [m.id,m]));
      }},
      async send(payload) {
        sent.push(payload); const id=String(500000000000000000n+BigInt(sequence++));
        const message={id,channelId:c.id,author:{id:config.botId},embeds:payload.embeds,components:payload.components,
          async edit(changes) { Object.assign(this,changes); return this; }};
        messages.set(id,message);
        if (w.failSend) { w.failSend=false; throw new Error('accepted but socket lost; private body'); }
        return message;
      },
    };
    channels.set(id,c); if (data.reason) audits.push({id:String(600000000000000000n+BigInt(sequence++)),user_id:config.botId,target_id:id,reason:data.reason}); return c;
  }
  const app=makeChannel({type:ChannelType.GuildCategory}); channels.delete(app.id); app.id=config.appCategoryId; channels.set(app.id,app);
  const bot={guild:{id:config.guildId},user:{bot:true},permissions:{has:p => p === P.Administrator},roles:{cache:new Set()}};
  members.set(config.botId,bot); members.set(requester,{...bot,user:{bot:false},permissions:{has:() => false}});
  members.set(staff,{...bot,user:{bot:false},permissions:{has:() => false},roles:{cache:new Set([config.supportRoleId])}});
  const guild={id:config.guildId,roles:{async fetch() { return new Collection([[config.supportRoleId,{id:config.supportRoleId,guild:{id:config.guildId},managed:false}]]); }},members:{async fetch({user,force,cache}) { assert.equal(force,true); assert.equal(cache,false); const m=members.get(user); if (!m) throw Object.assign(new Error('missing'),{code:10007}); return m; }},
    channels:{async fetch(id) { if (!id) return channels; const c=channels.get(id); if (!c) throw Object.assign(new Error('missing'),{code:10003}); return c; },async create(data) { const c=makeChannel(data); if (w.failCreate) { w.failCreate=false; throw new Error('accepted but connection lost'); } return c; }}};
  const client={user:{id:config.botId},application:{id:config.botId},guilds:{async fetch({guild: id,force}) { assert.equal(id,config.guildId); assert.equal(force,true); return guild; }},rest:{async get(route,{query}) { assert.equal(query.get('user_id'),config.botId); const before=query.get('before'); return {audit_log_entries:audits.filter(e => !before || BigInt(e.id)<BigInt(before)).slice().reverse().slice(0,100)}; }}};
  const adapter=new DiscordAdapter(client,f.store,config),tickets=new Tickets(f.store,adapter,config);
  const w={adapter,tickets,channels,members,client,sent,makeChannel}; return w;
}
test('real adapter bootstrap recovers ambiguous category/panel/message acceptance with bot ownership and no arbitrary name adoption',async t => {
  const f=fixture(t),w=world(f); const arbitrary=w.makeChannel({name:'🎫 Private Tickets',type:ChannelType.GuildCategory});
  w.failCreate=true; await assert.rejects(w.adapter.bootstrap()); assert.equal(f.store.resource('category').attempted,1); assert.equal(f.store.resource('category').id,null);
  await w.adapter.bootstrap(); assert.notEqual(w.adapter.category.id,arbitrary.id); const count=w.channels.size;
  await w.adapter.bootstrap(); assert.equal(w.channels.size,count); assert.equal(w.sent.length,1);
  // Simulate a crash immediately after accepted public panel-message delivery.
  f.store.db.prepare("UPDATE resources SET id=NULL WHERE key='panel-message'").run();
  await w.adapter.bootstrap(); assert.equal(w.sent.length,1);
});
test('real adapter recovers ambiguous private channel and initial-message success; claims and read-only closure work',async t => {
  const f=fixture(t),w=world(f); await w.adapter.bootstrap(); const input={...f.input,customId:w.tickets.modalId('300000000000000001',requester),subject:'@everyone hostile subject',description:'<@200000000000000099> test-only body'};
  w.failCreate=true; await assert.rejects(w.tickets.submit(input)); await w.tickets.reconcile();
  const ticket=f.store.all()[0]; assert.equal(ticket.state,'open'); assert.equal(w.channels.size,4); assert.deepEqual(w.sent.at(-1).allowedMentions,MENTIONS);
  assert.equal(w.sent.at(-1).enforceNonce,true);
  const ctx=f.context(ticket); await w.tickets.claim(ticket.id,ctx); const confirmation=await w.tickets.confirmClose(ticket.id,ctx);
  await w.tickets.close(confirmation,ctx); const c=w.channels.get(ticket.channel_id),requesterOverwrite=c.permissionOverwrites.cache.get(requester);
  assert.equal(requesterOverwrite.allow.bitfield&P.SendMessages,0n); assert.ok(requesterOverwrite.deny.bitfield&P.SendMessages);
  await w.tickets.reconcile(); assert.equal(w.sent.length,2);
  // Second independent request interrupted after accepted initial message.
  w.failSend=true; await assert.rejects(w.tickets.submit({...input,customId:w.tickets.modalId('300000000000000002',requester)}));
  await w.tickets.reconcile(); assert.equal(f.store.all().length,2); assert.equal(w.sent.length,3);
});
test('identity/config/privacy mismatches fail before Discord mutations; duplicate markers and foreign control messages fail closed',async t => {
  const f=fixture(t),w=world(f); w.client.user.id='100000000000000099'; await assert.rejects(w.adapter.bootstrap(),{code:'BOT_IDENTITY_MISMATCH'}); assert.equal(w.channels.size,1);
  w.client.user.id=config.botId; await w.adapter.bootstrap(); const input={...f.input,customId:w.tickets.modalId('300000000000000001',requester)}; const ticket=await w.tickets.submit(input);
  w.channels.get(ticket.channel_id).permissionOverwrites.cache.set('200000000000000099',{id:'200000000000000099',type:1,allow:{bitfield:P.ViewChannel},deny:{bitfield:0n}});
  await assert.rejects(w.tickets.reconcile(),{code:'PRIVACY_OVERWRITE_MISMATCH'});
  w.channels.get(ticket.channel_id).permissionOverwrites.cache.delete('200000000000000099');
  const message=await w.channels.get(ticket.channel_id).messages.fetch({message:ticket.message_id}); message.author.id=requester;
  await assert.rejects(w.tickets.claim(ticket.id,f.context(ticket)),{code:'CONTROL_MESSAGE_MISMATCH'}); message.author.id=config.botId;
  w.makeChannel({topic:w.adapter.marker('ticket',ticket.id),type:ChannelType.GuildText}); await assert.rejects(w.tickets.reconcile(),{code:'DUPLICATE_RESOURCE_MARKER'});
});
test('interaction handler defers privately before adapter work, checks source controls, and suppresses raw errors',async t => {
  const f=fixture(t),w=world(f); await w.adapter.bootstrap(); const calls=[];
  const modal={id:'300000000000000003',guildId:config.guildId,channelId:w.adapter.panel.id,user:{id:requester},customId:w.tickets.modalId('300000000000000003',requester),fields:{getTextInputValue:key => key === 'subject' ? 'fixture subject' : 'fixture body'},isButton:() => false,isModalSubmit:() => true,
    async deferReply(payload) { calls.push(['defer',payload]); this.deferred=true; },async editReply(payload) { calls.push(['edit',payload]); }};
  await handleInteraction(modal,w.tickets,w.adapter,() => true);
  assert.equal(calls[0][0],'defer'); assert.equal(calls[0][1].flags,MessageFlags.Ephemeral); assert.ok(calls[1][1].content.includes('private ticket'));
  const ticket=f.store.all()[0],button={...modal,user:{id:staff},channelId:ticket.channel_id,customId:`claim:${ticket.id}`,message:{id:'500000000000000099',author:{id:config.botId}},isButton:() => true,isModalSubmit:() => false};
  await handleInteraction(button,w.tickets,w.adapter,() => true); assert.equal(f.store.ticket(ticket.id).claimed_by,null); assert.ok(calls.at(-1)[1].content.includes('INTERACTION_SOURCE_INVALID'));
  w.tickets.submit=async () => { throw new Error('SECRET fixture token/private body'); };
  await handleInteraction(modal,w.tickets,w.adapter,() => true); assert.ok(!calls.at(-1)[1].content.includes('SECRET')); assert.deepEqual(calls.at(-1)[1].allowedMentions,MENTIONS);
});
test('missing Administrator, support role and App Support category type block bootstrap before writes',async t => {
  const f=fixture(t),w=world(f),bot=w.members.get(config.botId),permissions=bot.permissions;
  bot.permissions={has:() => false}; await assert.rejects(w.adapter.bootstrap(),{code:'BOT_ADMIN_REQUIRED'}); assert.equal(w.channels.size,1); bot.permissions=permissions;
  const roles=w.adapter.client.guilds.fetch; const guild=await roles({guild:config.guildId,force:true}),fetch=guild.roles.fetch;
  guild.roles.fetch=async () => new Collection(); await assert.rejects(w.adapter.bootstrap(),{code:'SUPPORT_ROLE_INVALID'}); assert.equal(w.channels.size,1); guild.roles.fetch=fetch;
  w.channels.get(config.appCategoryId).type=ChannelType.GuildText; await assert.rejects(w.adapter.bootstrap(),{code:'APP_CATEGORY_INVALID'}); assert.equal(w.channels.size,1);
});
test('staff authorization refreshes role definitions rather than granting stale Administrator permission',async t => {
  const f=fixture(t),w=world(f); await w.adapter.bootstrap(); const guild=await w.client.guilds.fetch({guild:config.guildId,force:true});
  let staleAdmin=true,refreshes=0; const actor=w.members.get(staff); actor.roles.cache.clear(); actor.permissions={has:permission => permission === P.Administrator && staleAdmin};
  guild.roles.fetch=async () => { refreshes++; staleAdmin=false; return new Collection(); };
  assert.equal(await w.adapter.isStaff(staff),false); assert.equal(refreshes,1);
});
