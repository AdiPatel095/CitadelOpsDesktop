import { test } from 'node:test';
import assert from 'node:assert/strict';
import { Store } from '../src/store.mjs';
import { Tickets } from '../src/tickets.mjs';
import { fixture,config,requester,staff,otherStaff } from './fixtures.mjs';
const code = expected => e => e.code === expected;
test('concurrent duplicate modal delivery opens exactly one ticket/chat/message; new requests are independent',async t => {
  const f=fixture(t); const results=await Promise.all(Array.from({length:20},() => f.tickets.submit(f.input)));
  assert.equal(new Set(results.map(t => t.id)).size,1); assert.equal(f.adapter.counts.create,1); assert.equal(f.adapter.counts.send,1);
  await Promise.all(Array.from({length:10},(_,i) => f.tickets.submit({...f.input,customId:f.tickets.modalId(String(300000000000000010n+BigInt(i)),requester)})));
  assert.equal(f.store.all().length,11); assert.equal(f.adapter.counts.create,11); assert.equal(f.adapter.counts.send,11);
  assert.ok(f.store.all().every(t => t.subject === null && t.description === null));
});
for (const step of ['afterCreate','afterSend']) test(`restart recovers accepted ${step} without duplicates, including modal replay`,async t => {
  const f=fixture(t); f.adapter.failures[step]=1;
  await assert.rejects(f.tickets.submit(f.input));
  assert.equal(f.store.all()[0].state,'creating'); assert.equal(f.store.all()[0].subject,'Test subject');
  f.store.close(); const reopened=new Store(f.dir); t.after(() => reopened.close()); const tickets=new Tickets(reopened,f.adapter,config);
  await tickets.reconcile(); const result=await tickets.submit(f.input);
  assert.equal(result.state,'open'); assert.equal(reopened.all().length,1); assert.equal(f.adapter.counts.create,1); assert.equal(f.adapter.counts.send,1);
});
test('absent outcome after attempted channel creation fails closed forever without another POST',async t => {
  const f=fixture(t); f.adapter.failures.beforeCreate=1; await assert.rejects(f.tickets.submit(f.input));
  await assert.rejects(f.tickets.reconcile(),code('CHANNEL_CREATION_UNCERTAIN')); await assert.rejects(f.tickets.submit(f.input),code('CHANNEL_CREATION_UNCERTAIN'));
  assert.equal(f.adapter.counts.create,1);
});
test('bound missing/mismatched channels and missing control message never create a replacement',async t => {
  const f=fixture(t); const ticket=await f.tickets.submit(f.input); f.adapter.channels.delete(ticket.id);
  await assert.rejects(f.tickets.reconcile(),code('TICKET_CHANNEL_MISMATCH')); assert.equal(f.adapter.counts.create,1);
  f.adapter.channels.set(ticket.id,{id:ticket.channel_id}); f.adapter.messages.delete(ticket.id);
  await assert.rejects(f.tickets.reconcile(),code('CONTROL_MESSAGE_MISMATCH')); assert.equal(f.adapter.counts.send,1);
});
test('claim uses fresh staff authorization, first-writer-wins and same-agent idempotency',async t => {
  const f=fixture(t),ticket=await f.tickets.submit(f.input),ctx=f.context(ticket);
  const results=await Promise.allSettled([f.tickets.claim(ticket.id,ctx),f.tickets.claim(ticket.id,{...ctx,actorId:otherStaff})]);
  assert.equal(results.filter(r => r.status === 'fulfilled').length,1); assert.equal(f.store.ticket(ticket.id).claimed_by,staff);
  await f.tickets.claim(ticket.id,ctx); f.adapter.authorized.delete(staff);
  await assert.rejects(f.tickets.claim(ticket.id,ctx),code('STAFF_REQUIRED'));
  await assert.rejects(f.tickets.claim(ticket.id,{...ctx,actorId:requester}),code('STAFF_REQUIRED'));
});
test('close requires actor-bound confirmation and revalidates current staff/channel/guild',async t => {
  const f=fixture(t),ticket=await f.tickets.submit(f.input),ctx=f.context(ticket);
  const id=await f.tickets.confirmClose(ticket.id,ctx);
  assert.ok(id.length <= 100);
  await assert.rejects(async () => f.tickets.close(id,{...ctx,actorId:otherStaff}),code('CONFIRMATION_INVALID'));
  await assert.rejects(async () => f.tickets.close(id,{...ctx,channelId:'400000000000000099'}),code('CONFIRMATION_INVALID'));
  await assert.rejects(f.tickets.claim(ticket.id,{...ctx,guildId:'100000000000000099'}),code('CONTROL_CONTEXT_INVALID'));
  f.adapter.authorized.delete(staff); await assert.rejects(f.tickets.close(id,ctx),code('STAFF_REQUIRED'));
  assert.equal(f.store.ticket(ticket.id).state,'open'); assert.equal(f.adapter.counts.readOnly,0);
});
for (const step of ['beforeClose','afterClose','update']) test(`durable close recovery after ${step} retains the conversation`,async t => {
  const f=fixture(t),ticket=await f.tickets.submit(f.input),ctx=f.context(ticket),id=await f.tickets.confirmClose(ticket.id,ctx);
  f.adapter.failures[step]=1; await assert.rejects(f.tickets.close(id,ctx));
  assert.equal(f.store.ticket(ticket.id).state,step === 'update' ? 'closed' : 'closing');
  await f.tickets.reconcile(); assert.equal(f.store.ticket(ticket.id).state,'closed'); assert.equal(f.adapter.channels.get(ticket.id).readonly,true);
  assert.equal(f.adapter.messages.get(ticket.id).controls,'closed'); assert.equal(f.adapter.counts.create,1);
  await f.tickets.close(id,ctx); await assert.rejects(f.tickets.claim(ticket.id,ctx),code('CONTROL_CONTEXT_INVALID'));
});
test('hostile modal IDs, inputs and requester binding are rejected; SQL-shaped input remains private data',async t => {
  const f=fixture(t);
  for (const input of [{...f.input,subject:' '},{...f.input,subject:'x'.repeat(101)},{...f.input,description:'x'.repeat(1501)},{...f.input,description:'\0'},{...f.input,customId:'ticket:bad'},{...f.input,requesterId:staff},{...f.input,guildId:'100000000000000099'}]) await assert.rejects(f.tickets.submit(input));
  assert.equal(f.store.all().length,0);
  const result=await f.tickets.submit({...f.input,subject:"'; DROP TABLE tickets; --",description:'@everyone <@&100000000000000003>'});
  assert.equal(result.state,'open'); assert.equal(f.store.all().length,1);
  const rows=f.store.db.prepare('SELECT * FROM audit').all(); assert.equal(rows.length,1); assert.equal(rows[0].action,'opened');
  assert.ok(!JSON.stringify(rows).includes('DROP TABLE'));
});
test('shutdown drains ongoing work and rejects new intake',async t => {
  const f=fixture(t); const pending=f.tickets.submit(f.input); await f.tickets.shutdown(); await pending;
  await assert.rejects(f.tickets.submit(f.input),code('SERVICE_NOT_READY'));
});
test('expired close confirmations cannot mutate ticket state',async t => {
  const f=fixture(t),ticket=await f.tickets.submit(f.input),ctx=f.context(ticket),id=await f.tickets.confirmClose(ticket.id,ctx),original=Date.now;
  try {
    Date.now=() => original()+121_000;
    await assert.rejects(async () => f.tickets.close(id,ctx),code('CONFIRMATION_INVALID'));
  } finally { Date.now=original; }
  assert.equal(f.store.ticket(ticket.id).state,'open'); assert.equal(f.adapter.counts.readOnly,0);
});
