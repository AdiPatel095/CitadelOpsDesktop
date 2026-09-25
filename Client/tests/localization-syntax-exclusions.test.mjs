import assert from 'node:assert/strict';
import test from 'node:test';
import ts from 'typescript';
import {createSyntaxExclusionReviewer} from '../scripts/source-syntax-exclusions.mjs';
function inspect(text){const source=ts.createSourceFile('fixture.tsx',text,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX);const review=createSyntaxExclusionReviewer(source,new Set(['battle.sourceNotLoaded']));const result=[];function walk(node){if(ts.isStringLiteral(node)||ts.isNoSubstitutionTemplateLiteral(node))result.push({text:node.text,reason:review(node)});ts.forEachChild(node,walk);}walk(source);return result;}
test('SVG rendering exclusions preserve titles, descriptions, aria and custom component props',()=>{
 const rows=inspect('const x=<svg fill="none"><path strokeLinecap="round" d="M0 0" aria-label="Visible path"/><title>{"Visible title"}</title><desc>{"Visible description"}</desc><Custom fill="Visible custom prop"/></svg>');
 for(const text of ['none','round','M0 0'])assert.ok(rows.find(row=>row.text===text).reason);
 for(const text of ['Visible path','Visible title','Visible description','Visible custom prop'])assert.equal(rows.find(row=>row.text===text).reason,null);
 assert.equal(inspect('const x=<path fill="Visible outside SVG"/>')[0].reason,null);
});
test('only known keys in imported MessageKey state qualify; arbitrary key objects and string state do not',()=>{
 const rows=inspect('import {useState} from "react"; import type {MessageKey} from "../i18n/messages"; const a=useState<MessageKey>("battle.sourceNotLoaded"); const b=useState<string>("Visible state"); const c={key:"battle.sourceNotLoaded"}; const d=useState<MessageKey>("Unknown key");');
 assert.ok(rows.filter(row=>row.text==='battle.sourceNotLoaded')[0].reason);
 assert.equal(rows.filter(row=>row.text==='battle.sourceNotLoaded')[1].reason,null);
 for(const text of ['Visible state','Unknown key'])assert.equal(rows.find(row=>row.text===text).reason,null);
 assert.equal(inspect('type MessageKey=string; function useState(){}; const a=useState<MessageKey>("battle.sourceNotLoaded");').at(-1).reason,null);
});
