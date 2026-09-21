import test from 'node:test';
import assert from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { renderRichMessage, richMessageContract, validateRichMessageCatalog } from '../src/i18n/RichMessage.ts';
const source={key:'privacy',fallback:'Read our <privacy>Privacy Policy</privacy>, {name}.',params:{name:'<script>alert(1)</script>'}};
const tags={privacy:children=>createElement('a',{href:'/privacy',rel:'help'},...children)};
test('whole sentences can reorder safe React links and scalar values',()=>{
 const result=renderRichMessage(source,'de',{privacy:'{name}, lies unsere <privacy>Datenschutzerklärung</privacy>.'},tags);
 const html=renderToStaticMarkup(result.content);
 assert.equal(result.translated,true);
 assert.match(html,/&lt;script&gt;/);
 assert.match(html,/<a href="\/privacy" rel="help">Datenschutzerklärung<\/a>/);
 assert.equal(html.includes('<script>'),false);
});
test('missing/extra tags, attributes and scalar mismatch reject translation',()=>{
 for(const text of ['Read <evil>here</evil>, {name}.','Read here, {name}.','<privacy href="https://evil">Here</privacy> {name}','<privacy>Here</privacy> {missing}']){
  const result=renderRichMessage(source,'de',{privacy:text},tags);
  assert.equal(result.translated,false);
  assert.match(renderToStaticMarkup(result.content),/Privacy Policy/);
 }
});
test('contract validator checks nested plural tags and argument parity',()=>{
 const text='{count, plural, one {<link>One</link>} other {<link># items</link>}}';
 const contract=richMessageContract(text);
 assert.deepEqual(contract,{arguments:['count'],tags:['link']});
 assert.deepEqual(validateRichMessageCatalog({x:text},{x:text},{x:contract}),[]);
 assert.equal(validateRichMessageCatalog({x:text},{x:'{count}'},{x:contract}).length,1);
});
test('invalid caller tags never produce arbitrary markup',()=>{
 const result=renderRichMessage(source,'en',{},{});
 assert.equal(result.translated,false);
 assert.match(renderToStaticMarkup(result.content),/&lt;privacy&gt;/);
});
test('Arabic rich argument isolation preserves selector values and user braces',()=>{
 const message={key:'x',fallback:'<name>{user}</name>: {kind, select, player {player} other {other}}',params:{user:'Player-7 {x}',kind:'player'}};
 const result=renderRichMessage(message,'ar',{x:'<name>{user}</name>: {kind, select, player {لاعب} other {آخر}}'},{name:children=>createElement('b',null,...children)});
 assert.equal(result.translated,true);
 assert.match(renderToStaticMarkup(result.content),/\u2068Player-7 \{x\}\u2069/);
 assert.match(renderToStaticMarkup(result.content),/لاعب/);
});
test('project rich source templates match explicit scalar and tag contracts',async()=>{
 const {richMessages,richContracts}=await import('../src/i18n/richMessages.ts');
 assert.deepEqual(validateRichMessageCatalog(richMessages,richMessages,richContracts),[]);
 for(const [key,fallback] of Object.entries(richMessages)) {
  const contract=richContracts[key];
  const params=Object.fromEntries(contract.arguments.map(name=>[name,name === 'cost' ? 2500 : 'protocol_{unchanged}']));
  const callbacks=Object.fromEntries(contract.tags.map(name=>[name,children=>createElement('b',{key:name},...children)]));
  const result=renderRichMessage({key,fallback,params},'en',{},callbacks);
  const html=renderToStaticMarkup(result.content);
  assert.equal(result.translated,false);
  if(contract.arguments.some(name=>name!=='cost'))assert.match(html,/protocol_\{unchanged\}/);
  assert.equal(html.includes('&lt;span0&gt;'),false,key);
 }
});
