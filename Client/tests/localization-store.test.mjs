import test from 'node:test';
import assert from 'node:assert/strict';
import {readViewerLocale,readViewerLocalePreference,resolveDeviceLocale,setViewerLocale,subscribeViewerLocale,viewerLocaleEvent} from '../src/i18n/viewerLocaleStore.ts';
import {localeStorageKey} from '../src/i18n/locales.ts';

const sessionKey=Symbol.for('citadelops.viewer-locale.session');
function environment(run) {
 const descriptors=Object.fromEntries(['window','navigator','localStorage'].map(key=>[key,Object.getOwnPropertyDescriptor(globalThis,key)]));
 const values=new Map(); const device={languages:['xx','de-DE'],language:'fr'};
 Object.defineProperties(globalThis,{window:{configurable:true,value:new EventTarget()},navigator:{configurable:true,value:device},localStorage:{configurable:true,value:{getItem:key=>values.get(key)??null,setItem:(key,value)=>values.set(key,value),removeItem:key=>values.delete(key)}}});
 globalThis[sessionKey].preference=undefined;
 const storage=(key,newValue,storageArea)=>{const event=new Event('storage');Object.defineProperties(event,{key:{value:key},newValue:{value:newValue},storageArea:{value:storageArea}});window.dispatchEvent(event);};
 try {run({values,device,storage});} finally {
  globalThis[sessionKey].preference=undefined;
  for(const [key,descriptor] of Object.entries(descriptors)) {if(descriptor)Object.defineProperty(globalThis,key,descriptor);else delete globalThis[key];}
 }
}
test('automatic resolves supported language order, aliases, scripts and fallback',()=>environment(({device,values})=>{
 assert.equal(readViewerLocalePreference(),'auto');assert.equal(readViewerLocale(),'de');
 for(const [languages,expected] of [[['zh-MO'],'zh-TW'],[['zh-Hans-HK'],'zh-CN'],[['nb-NO'],'no'],[['nn'],'no'],[['be','af','fr-CA'],'fr']]){device.languages=languages;assert.equal(resolveDeviceLocale(),expected);}
 device.languages=['unknown'];device.language='ja-JP';assert.equal(readViewerLocale(),'ja');
 device.language='unknown';assert.equal(readViewerLocale(),'en');
 values.set(localeStorageKey,'garbage');assert.equal(readViewerLocalePreference(),'auto');
}));
test('explicit preferences persist/reload and automatic removes saved override',()=>environment(({values,device})=>{
 setViewerLocale('ar');assert.equal(values.get(localeStorageKey),'ar');assert.equal(readViewerLocale(),'ar');
 globalThis[sessionKey].preference=undefined;assert.equal(readViewerLocalePreference(),'ar');
 device.languages=['fr'];assert.equal(readViewerLocale(),'ar');
 setViewerLocale('auto');assert.equal(values.has(localeStorageKey),false);assert.equal(readViewerLocale(),'fr');
 globalThis[sessionKey].preference=undefined;assert.equal(readViewerLocale(),'fr');
}));
test('cross-tab reset/clear/invalid preference and languagechange respect auto mode',()=>environment(({storage,device})=>{
 let notifications=0;const stop=subscribeViewerLocale(()=>notifications++);
 try {
  storage(localeStorageKey,'es');assert.equal(readViewerLocale(),'es');
  window.dispatchEvent(new Event('languagechange'));assert.equal(notifications,1);
  storage('unrelated','ru');storage(localeStorageKey,'ru',{});assert.equal(notifications,1);
  storage(localeStorageKey,null);assert.equal(readViewerLocale(),'de');
  device.languages=['fr'];window.dispatchEvent(new Event('languagechange'));assert.equal(readViewerLocale(),'fr');assert.equal(notifications,3);
  storage(null,null);assert.equal(readViewerLocalePreference(),'auto');
  storage(localeStorageKey,'invalid');assert.equal(readViewerLocalePreference(),'auto');
  window.dispatchEvent(new CustomEvent(viewerLocaleEvent,{detail:'ar'}));assert.equal(readViewerLocale(),'ar');
  setViewerLocale('unsupported');assert.equal(readViewerLocale(),'ar');
  stop();const count=notifications;setViewerLocale('de');window.dispatchEvent(new Event('languagechange'));assert.equal(notifications,count);
 } finally {stop();}
}));
test('blocked storage preserves explicit and automatic in-memory selections',()=>environment(({device})=>{
 Object.defineProperty(globalThis,'localStorage',{configurable:true,get(){throw Error('blocked');}});
 assert.equal(readViewerLocale(),'de');setViewerLocale('ar');assert.equal(readViewerLocale(),'ar');
 setViewerLocale('auto');device.languages=['fr'];assert.equal(readViewerLocale(),'fr');assert.equal(readViewerLocalePreference(),'auto');
}));
