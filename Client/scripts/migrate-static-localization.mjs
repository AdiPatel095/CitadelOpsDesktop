import ts from 'typescript';
import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
const sourceRoot = new URL('../src/',import.meta.url).pathname;
const apply = process.argv.includes('--apply');
if (apply && fs.existsSync(new URL('../src/i18n/sourceMessages.ts',import.meta.url))) throw new Error('This reviewed migration already ran; do not overwrite its catalog or manifest.');
const messages = {};
const migrations = [];
const exclusions = [];
const prohibited = new Set(['pre','code','textarea','option','script','style','svg','title','desc','LocalizedText']);
const entities = {'&amp;':'&','&lt;':'<','&gt;':'>','&quot;':'"','&apos;':"'",'&nbsp;':'\u00a0'};
const brand = /^(Citadel\s?Ops|Citadel|Discord|Goodgame Empire|JSON|API|HTTP|HTTPS|CLI|RPC|ID|OK|XP|HP|AM|PM)$/;
function visitDirectory(directory) {
 for (const name of fs.readdirSync(directory).sort()) {
  const filename=path.join(directory,name);
  if(fs.statSync(filename).isDirectory()){visitDirectory(filename);continue;}
  if(!name.endsWith('.tsx'))continue;
  const relative=path.relative(sourceRoot,filename);
  if(relative.startsWith('i18n/'))continue;
  const text=fs.readFileSync(filename,'utf8');
  const source=ts.createSourceFile(filename,text,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX);
  const edits=[];
  function visit(node){
   if(ts.isJsxText(node) && node.text.trim()) {
    const parent=node.parent;
    const location={file:relative,line:source.getLineAndCharacterOfPosition(node.getStart(source)).line+1};
    let reason;
    let ancestor=parent;
    while(ancestor){
     if(ts.isJsxElement(ancestor) && prohibited.has(ancestor.openingElement.tagName.getText(source)))reason='technical-or-special-HTML-content';
     ancestor=ancestor.parent;
    }
    const children=parent.children ? parent.children.filter(child=>!ts.isJsxText(child)||child.text.trim()) : [];
    if(children.length!==1)reason='whole-sentence-rich-or-expression-review';
    const raw=node.text.replace(/\s+/g,' ').trim();
    if(!/[A-Za-z]{2}/.test(raw)||brand.test(raw)||/^([A-Z0-9_./:+-]{1,8}|https?:\/\/\S+)$/.test(raw))reason='technical-token-or-brand';
    if(/[{}]/.test(raw))reason='literal-brace-review';
    if(/&[A-Za-z0-9#]+;/.test(raw.replace(/&(amp|lt|gt|quot|apos|nbsp);/g,'')))reason='entity-review';
    if(reason){exclusions.push({...location,reason});} else {
     const value=raw.replace(/&(amp|lt|gt|quot|apos|nbsp);/g,match=>entities[match]);
     const namespace=relative.replace(/\.tsx$/,'').split('/').map(part=>part[0].toLowerCase()+part.slice(1)).join('.');
     const slug=value.toLowerCase().replace(/[^a-z0-9]+/g,'.').replace(/^\.|\.$/g,'').split('.').slice(0,7).join('.');
     const key=`ui.${namespace}.${slug}.${createHash('sha256').update(value).digest('hex').slice(0,8)}`;
     messages[key]=value;
     migrations.push({...location,key,source:value,kind:'standalone-jsx-text'});
     edits.push({start:node.getStart(source),end:node.end,text:`<LocalizedText messageKey=${JSON.stringify(key)} />`});
    }
   }
   ts.forEachChild(node,visit);
  }
  visit(source);
  if(apply&&edits.length){
   let next=text;
   for(const edit of edits.sort((a,b)=>b.start-a.start))next=next.slice(0,edit.start)+edit.text+next.slice(edit.end);
   let importPath=path.relative(path.dirname(filename),path.join(sourceRoot,'i18n/LocalizedText')).split(path.sep).join('/');
   if(!importPath.startsWith('.'))importPath='./'+importPath;
   next=`import { LocalizedText } from ${JSON.stringify(importPath)};\n`+next;
   fs.writeFileSync(filename,next);
  }
 }
}
visitDirectory(sourceRoot);
const byFile={};for(const row of migrations)byFile[row.file]=(byFile[row.file]??0)+1;
const byReason={};for(const row of exclusions)byReason[row.reason]=(byReason[row.reason]??0)+1;
if(apply){
 fs.writeFileSync(new URL('../src/i18n/sourceMessages.ts',import.meta.url),`/** Explicit source-assigned static text keys. Missing locale entries remain English fallback. */\nexport const sourceMessages = ${JSON.stringify(messages,null,2)} as const;\n`);
 fs.writeFileSync(new URL('../localization/static-migrations.json',import.meta.url),JSON.stringify({migrations,exclusions})+'\n');
}
console.log(JSON.stringify({mode:apply?'apply':'preview',keys:Object.keys(messages).length,sinks:migrations.length,files:Object.keys(byFile).length,excluded:byReason,byFile}));
