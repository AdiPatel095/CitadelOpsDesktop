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
