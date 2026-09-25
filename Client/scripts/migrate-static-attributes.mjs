import ts from 'typescript';
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {sourceMessages} from '../src/i18n/sourceMessages.ts';
const root=new URL('../src/',import.meta.url).pathname;
const apply=process.argv.includes('--apply');
const manifestURL=new URL('../localization/attribute-migrations.json',import.meta.url);
if(apply&&fs.existsSync(manifestURL))throw new Error('Reviewed attribute migration already ran; preserve its manifest.');
const attributes=new Set(['title','placeholder','aria-label','aria-description','alt','label','description','help','tooltip','emptyText','heading','ariaLabel','searchPlaceholder','emptyMessage','eyebrow']);
const messages={...sourceMessages};const migrations=[];const exclusions=[];
function componentOwner(node,source){
 for(let ancestor=node.parent;ancestor;ancestor=ancestor.parent){
  if(!ts.isArrowFunction(ancestor)&&!ts.isFunctionExpression(ancestor)&&!ts.isFunctionDeclaration(ancestor))continue;
  if(!ancestor.body||!ts.isBlock(ancestor.body))continue;
  let name=ancestor.name?.getText(source);
  if(!name){
   let holder=ancestor.parent;
   while(holder&&(ts.isCallExpression(holder)||ts.isParenthesizedExpression(holder)))holder=holder.parent;
   if(holder&&ts.isVariableDeclaration(holder))name=holder.name.getText(source);
  }
  if(name&&/^[A-Z][A-Za-z0-9_]*$/.test(name))return ancestor;
 }
}
function walk(dir){for(const name of fs.readdirSync(dir).sort()){
 const file=path.join(dir,name);if(fs.statSync(file).isDirectory()){walk(file);continue;}
 const relative=path.relative(root,file);if(!name.endsWith('.tsx')||relative.startsWith('i18n/'))continue;
 const text=fs.readFileSync(file,'utf8');const source=ts.createSourceFile(file,text,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX);
 const edits=[];const owners=new Set();
 function visit(node){
  if(ts.isJsxAttribute(node)&&attributes.has(node.name.getText(source))&&node.initializer&&ts.isStringLiteral(node.initializer)){
   const value=node.initializer.text;const attribute=node.name.getText(source);const owner=componentOwner(node,source);
   let reason;
   if(!/[A-Za-z]{2}/.test(value)||/^(https?:\/\/\S+|[A-Z0-9_.:/+-]{1,8})$/.test(value))reason='technical-token-or-example';
   if(/[{}]/.test(value))reason='literal-brace-review';
   if(!owner)reason='noncomponent-or-expression-body-requires-review';
   const location={file:relative,line:source.getLineAndCharacterOfPosition(node.getStart(source)).line+1,attribute};
   if(reason)exclusions.push({...location,reason});else{
    const namespace=relative.replace(/\.tsx$/,'').split('/').map(part=>part[0].toLowerCase()+part.slice(1)).join('.');
    const slug=value.toLowerCase().replace(/[^a-z0-9]+/g,'.').replace(/^\.|\.$/g,'').split('.').slice(0,7).join('.');
    const key=`ui.${namespace}.${attribute}.${slug}.${createHash('sha256').update(value).digest('hex').slice(0,8)}`;
    messages[key]=value;migrations.push({...location,key,source:value,kind:'static-visible-attribute'});
    edits.push({start:node.initializer.getStart(source),end:node.initializer.end,text:`{localizeStatic(${JSON.stringify(key)})}`});owners.add(owner);
   }
  }
  ts.forEachChild(node,visit);
 }
 visit(source);
 if(apply&&edits.length){
  if(/\b(localizeStatic|useStaticLocale)\b/.test(text))throw new Error(`Reserved migration binding already exists: ${relative}`);
  for(const owner of owners)edits.push({start:owner.body.getStart(source)+1,end:owner.body.getStart(source)+1,text:'\n  const { t: localizeStatic } = useStaticLocale();'});
  let next=text;for(const edit of edits.sort((a,b)=>b.start-a.start))next=next.slice(0,edit.start)+edit.text+next.slice(edit.end);
  let importPath=path.relative(path.dirname(file),path.join(root,'i18n/LocaleContext')).split(path.sep).join('/');if(!importPath.startsWith('.'))importPath='./'+importPath;
  next=`import { useLocale as useStaticLocale } from ${JSON.stringify(importPath)};\n`+next;fs.writeFileSync(file,next);
 }
}}
walk(root);
if(apply){fs.writeFileSync(new URL('../src/i18n/sourceMessages.ts',import.meta.url),`/** Explicit source-assigned messages; missing locale entries are English fallback. */\nexport const sourceMessages = ${JSON.stringify(messages,null,2)} as const;\n`);fs.writeFileSync(manifestURL,JSON.stringify({migrations,exclusions})+'\n');}
console.log(JSON.stringify({mode:apply?'apply':'preview',keys:new Set(migrations.map(row=>row.key)).size,sinks:migrations.length,files:new Set(migrations.map(row=>row.file)).size,excluded:exclusions.length,byReason:Object.fromEntries([...new Set(exclusions.map(row=>row.reason))].map(reason=>[reason,exclusions.filter(row=>row.reason===reason).length]))}));
