import assert from 'node:assert/strict';
import test from 'node:test';
import {battleLaneMessageKey} from '../src/battleStats/utils/BattleLaneIdentity.ts';
test('only documented lane identities select translations',()=>{
 assert.equal(battleLaneMessageKey('Left flank',2),'battle.leftFlank');
 assert.equal(battleLaneMessageKey('',2),'battle.rightFlank');
 for(const literal of ['constructor','toString','__proto__','Historic custom lane','left flank'])assert.equal(battleLaneMessageKey(literal,0),undefined,literal);
 assert.equal(battleLaneMessageKey('',3),undefined);
});

import {formatLegacyBattleEffectPercent} from '../src/battleStats/utils/BattleEffectNumber.ts';
test('legacy effect display retains percent units and one decimal across locales',()=>{
 assert.equal(formatLegacyBattleEffectPercent(12.34,'en'),'+12.3%');
 assert.equal(formatLegacyBattleEffectPercent(-12.34,'de'),'-12,3 %');
 assert.match(formatLegacyBattleEffectPercent(-12.34,'ar-u-nu-arab'),/١٢٫٣/);
 assert.equal(formatLegacyBattleEffectPercent(0,'en'),'0.0%');
});
