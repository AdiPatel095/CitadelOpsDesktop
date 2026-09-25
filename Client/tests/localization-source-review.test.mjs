import test from 'node:test';
import assert from 'node:assert/strict';
import {sourceCandidateIdentity,applySourceReviews} from '../scripts/source-review.mjs';
const id=sourceCandidateIdentity('api.ts','StringLiteral','wire','dispatch:request("wire")');
const entry={id,category:'requires-dataflow-review',classification:'unreviewed'};
const review={classification:'technical',reason:'Protocol command discriminator',evidence:'api.ts dispatch passes this value as commandID, never a display label'};
test('source-bound reviews survive line drift but reject changed source or context',()=>{
 assert.equal(applySourceReviews([{...entry,line:1}],{[id]:review}).entries[0].classification,'technical');
 assert.equal(applySourceReviews([{...entry,line:50}],{[id]:review}).errors.length,0);
 for(const next of [sourceCandidateIdentity('api.ts','StringLiteral','new','dispatch:request("new")'),sourceCandidateIdentity('api.ts','StringLiteral','wire','render:label("wire")')])assert.match(applySourceReviews([{...entry,id:next}],{[id]:review}).errors[0],/Stale/);
});
test('review records cannot waive visible prose or omit reachability evidence',()=>{
 assert.match(applySourceReviews([{...entry,category:'visible-jsx-text'}],{[id]:review}).errors[0],/key conversion/);
 assert.match(applySourceReviews([entry],{[id]:{classification:'technical',reason:'ID'}}).errors[0],/Invalid/);
 assert.match(applySourceReviews([entry],{[id]:{...review,classification:'translated'}}).errors[0],/Invalid/);
});
