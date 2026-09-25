import assert from 'node:assert/strict';
import test from 'node:test';
import {metadataName,translationValues} from '../src/i18n/officialMetadata.ts';
test('viewer official names precede locale-neutral English display names',()=>{
 const row={name:'wood',_display_name:'Wood'};
 assert.deepEqual(metadataName(row,translationValues({values:{wood:'Holz'},resolvedLocale:'de'}),'Resource'),{name:'Holz',nameLocale:'de',localizationKey:'wood',translationStatus:'official'});
 assert.equal(row._display_name,'Wood');
});
test('English and absent official keys remain explicit fallback provenance',()=>{
 const row={name:'wood',_display_name:'Wood'};
 assert.equal(metadataName(row,translationValues({values:{wood:'Wood'},resolvedLocale:'de',fallbackKeys:['wood']}),'Resource').nameLocale,'en');
 assert.deepEqual(metadataName(row,{},'Resource'),{name:'Wood',nameLocale:'en',translationStatus:'fallback'});
});
