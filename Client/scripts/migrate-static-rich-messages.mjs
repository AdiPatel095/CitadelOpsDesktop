import ts from 'typescript';
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {sourceMessages} from '../src/i18n/sourceMessages.ts';
import {officialMessageKeys} from '../src/i18n/officialKeys.ts';
const root=new URL('../src/',import.meta.url).pathname;
const apply=process.argv.includes('--apply');
const richURL=new URL('../src/i18n/richMessages.ts',import.meta.url);
if(apply&&fs.existsSync(richURL))throw new Error('Reviewed rich migration already ran.');
const inline=new Set(['strong','em','b','i','a','code','span']);
const messages={};const contracts={};const migrations=[];
function walk(dir){for(const name of fs.readdirSync(dir).sort()){
 const file=path.join(dir,name);if(fs.statSync(file).isDirectory()){walk(file);continue;}
 const relative=path.relative(root,file);if(!name.endsWith('.tsx')||relative.startsWith('i18n/'))continue;
 const text=fs.readFileSync(file,'utf8');const source=ts.createSourceFile(file,text,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX);const edits=[];
 function candidate(parent){
  const tags={};const params={};const oldKeys=[];let sequence=0;
  function children(nodes){return nodes.map(node=>{
   if(ts.isJsxText(node))return node.text.replace(/\s+/g,' ');
   if(ts.isJsxExpression(node)&&node.expression&&ts.isStringLiteral(node.expression))return node.expression.text;
   if(ts.isJsxSelfClosingElement(node)&&node.tagName.getText(source)==='LocalizedText'){
    const attr=node.attributes.properties.find(attr=>ts.isJsxAttribute(attr)&&attr.name.getText(source)==='messageKey');const key=attr?.initializer?.text;
    if(!key||!Object.hasOwn(sourceMessages,key)||Object.hasOwn(officialMessageKeys,key))throw Error('nonstatic-text');oldKeys.push(key);return sourceMessages[key];
   }
   if(ts.isJsxElement(node)&&inline.has(node.openingElement.tagName.getText(source))){
    const tagName=node.openingElement.tagName.getText(source);const tag=`${tagName}${sequence++}`;
    const inner=children(node.children);
    tags[tag]=`children => ${node.openingElement.getText(source)}{children}${node.closingElement.getText(source)}`;
    const className=node.openingElement.attributes.properties.find(attr=>ts.isJsxAttribute(attr)&&attr.name.getText(source)==='className')?.initializer?.text ?? '';
    if(tagName==='code'||className.split(/\s+/).includes('font-mono')){
     if(/[<>]/.test(inner))throw Error('nested-code');
     const param=`codeText${Object.keys(params).length}`;params[param]=inner.trim();return `<${tag}>{${param}}</${tag}>`;
    }
    return `<${tag}>${inner}</${tag}>`;
   }
   throw Error('dynamic-or-block-content');
  }).join('');}
  try{
   const value=children(parent.children).trim();
   if(!/[A-Za-z]{2}/.test(value)||!Object.keys(tags).length||/&[A-Za-z#0-9]+;/.test(value))return;
   // Braces are allowed only for the explicitly preserved code values.
   if(/[{}]/.test(value.replace(/\{codeText\d+\}/g,'')))return;
   const namespace=relative.replace(/\.tsx$/,'').split('/').map(part=>part[0].toLowerCase()+part.slice(1)).join('.');
   const slug=value.replace(/<[^>]*>/g,'').toLowerCase().replace(/[^a-z0-9]+/g,'.').replace(/^\.|\.$/g,'').split('.').slice(0,7).join('.');
   const key=`ui.rich.${namespace}.${slug}.${createHash('sha256').update(value).digest('hex').slice(0,8)}`;
   return {key,value,tags,params,oldKeys};
  }catch{return;}
 }
 function visit(node){
  if(ts.isJsxElement(node)&&!['pre','code','textarea','option','script','style','svg'].includes(node.openingElement.tagName.getText(source))&&node.children.some(child=>ts.isJsxText(child)&&child.text.trim())&&node.children.some(ts.isJsxElement)){
   const found=candidate(node);
   if(found){
    messages[found.key]=found.value;contracts[found.key]={arguments:Object.keys(found.params).sort(),tags:Object.keys(found.tags).sort()};
    migrations.push({file:relative,line:source.getLineAndCharacterOfPosition(node.getStart(source)).line+1,key:found.key,source:found.value,replacesStaticKeys:found.oldKeys,kind:'whole-static-rich-sentence'});
    const tags=Object.entries(found.tags).map(([key,value])=>`${key}: ${value}`).join(', ');
    edits.push({start:node.openingElement.end,end:node.closingElement.getStart(source),text:`<LocalizedRichText messageKey=${JSON.stringify(found.key)} params={${JSON.stringify(found.params)}} tags={{${tags}}} />`});return;
   }
  }
  ts.forEachChild(node,visit);
 }
 visit(source);
 if(apply&&edits.length){let next=text;for(const edit of edits.sort((a,b)=>b.start-a.start))next=next.slice(0,edit.start)+edit.text+next.slice(edit.end);let importPath=path.relative(path.dirname(file),path.join(root,'i18n/LocalizedRichText')).split(path.sep).join('/');if(!importPath.startsWith('.'))importPath='./'+importPath;fs.writeFileSync(file,`import { LocalizedRichText } from ${JSON.stringify(importPath)};\n`+next);}
}}
walk(root);
if(apply){fs.writeFileSync(richURL,`/** Whole-sentence templates; only application-owned React callbacks render tags. */\nexport const richMessages = ${JSON.stringify(messages,null,2)} as const;\nexport const richContracts = ${JSON.stringify(contracts,null,2)} as const;\n`);fs.writeFileSync(new URL('../localization/rich-migrations.json',import.meta.url),JSON.stringify(migrations)+'\n');}
console.log(JSON.stringify({mode:apply?'apply':'preview',keys:Object.keys(messages).length,sinks:migrations.length,files:new Set(migrations.map(row=>row.file)).size,migrations}));
