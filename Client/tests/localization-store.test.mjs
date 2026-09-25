import test from 'node:test';
import assert from 'node:assert/strict';
import {readViewerLocale,setViewerLocale,subscribeViewerLocale,viewerLocaleEvent} from '../src/i18n/viewerLocaleStore.ts';
import {localeStorageKey} from '../src/i18n/locales.ts';

test('same-tab selection survives storage failure and syncs other provider instances',()=>{
 const previousWindow=globalThis.window;
 globalThis.window=new EventTarget();
 let notifications=0;
 const stop=subscribeViewerLocale(()=>notifications++);
 try {
  setViewerLocale('ar');
  assert.equal(readViewerLocale(),'ar');
  assert.equal(notifications,1);
  window.dispatchEvent(new CustomEvent(viewerLocaleEvent,{detail:'fr'}));
  assert.equal(readViewerLocale(),'fr');
  const event=new Event('storage');Object.defineProperties(event,{key:{value:localeStorageKey},newValue:{value:null}});window.dispatchEvent(event);
  assert.equal(readViewerLocale(),'en');
  setViewerLocale('unsupported');assert.equal(readViewerLocale(),'en');
  stop();const count=notifications;setViewerLocale('de');assert.equal(notifications,count);
 } finally {stop();globalThis.window=previousWindow;}
});
