import test from 'node:test';
import assert from 'node:assert/strict';
import { typographyFailures } from '../scripts/typography/check.mjs';

test('zero ratchet rejects raw CSS sizes, weights and font shorthand',()=>{
  assert.equal(typographyFailures('.a { font-size:11px; font-weight:800; font:12px sans-serif; }','a.css').length,3);
  assert.deepEqual(typographyFailures('.a { font-size:var(--font-size-12); font-weight:var(--font-weight-600); }','a.css'),[]);
  assert.equal(typographyFailures('.a { font-size:var(--other); font-weight:var(--other); }','a.css').length,2);
});
test('zero ratchet covers class constants, dynamic templates and SVG text',()=>{
  assert.equal(typographyFailures('const c = `text-[11px] ${active}`; const x=<text fontSize={11}/>;','a.tsx').length,2);
  assert.deepEqual(typographyFailures('const c="text-caption font-semibold"; const x=<text fontSize="var(--font-size-12)"/>;','a.tsx'),[]);
  assert.equal(typographyFailures('const c="text-sm text-body/3 font-black";','a.tsx').length,3);
  assert.equal(typographyFailures('const c="font-[800]";','a.tsx').length,1);
});
test('locale-aware eyebrow tokens are allowed; raw uppercase and tracking fail',()=>{
  assert.deepEqual(typographyFailures('.eyebrow { text-transform:var(--eyebrow-case); letter-spacing:var(--tracking-eyebrow); }','a.css'),[]);
  assert.equal(typographyFailures('.label { text-transform:uppercase; letter-spacing:.06em; }','a.css').length,2);
});
