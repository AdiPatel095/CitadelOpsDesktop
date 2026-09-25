import ts from 'typescript';
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {sourceMessages} from '../src/i18n/sourceMessages.ts';
const root=new URL('../src/',import.meta.url).pathname;
const apply=process.argv.includes('--apply');
const manifestURL=new URL('../localization/icon-label-migrations.json',import.meta.url);
if(apply&&fs.existsSync(manifestURL))throw new Error('Reviewed icon-label migration already applied');
const messages={...sourceMessages},migrations=[];
function walk(dir){for(const name of fs.readdirSync(dir).sort()){
 const file=path.join(dir,name);if(fs.statSync(file).isDirectory()){walk(file);continue;}
 const relative=path.relative(root,file);if(!name.endsWith('.tsx')||relative.startsWith('i18n/'))continue;
 const text=fs.readFileSync(file,'utf8'),source=ts.createSourceFile(file,text,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX),icons=new Set();
 for(const statement of source.statements)if(ts.isImportDeclaration(statement)&&statement.moduleSpecifier.text==='lucide-react')for(const specifier of statement.importClause?.namedBindings?.elements??[])icons.add(specifier.name.text);
 const edits=[];
 function visit(node){
  if(ts.isJsxElement(node)&&!['pre','code','textarea','option','script','style','svg','title','desc'].includes(node.openingElement.tagName.getText(source))) {
   const children=node.children.filter(child=>!ts.isJsxText(child)||child.text.trim());
   const texts=children.filter(ts.isJsxText);
   if(texts.length===1&&children.length>1&&children.every(child=>ts.isJsxText(child)||(ts.isJsxSelfClosingElement(child)&&(icons.has(child.tagName.getText(source))||/^Icons\.[A-Za-z]+$/.test(child.tagName.getText(source)))))) {
    const item=texts[0],raw=item.text.replace(/\s+/g,' ').trim();
    if(/[A-Za-z]{2}/.test(raw)&&!/[{}&]/.test(raw)&&!/^(Citadel\s?Ops|Citadel|Discord|JSON|API|HTTP|ID|XP|HP|LID|MID|CRA)$/.test(raw)) {
     const namespace=relative.replace(/\.tsx$/,'').split('/').map(part=>part[0].toLowerCase()+part.slice(1)).join('.');
     const slug=raw.toLowerCase().replace(/[^a-z0-9]+/g,'.').replace(/^\.|\.$/g,'').split('.').slice(0,7).join('.');
     // Reviewed action semantics: these icon-adjacent Refresh buttons reload their current data.
     const key=raw==='Refresh'?'common.refresh':`ui.${namespace}.${slug}.${createHash('sha256').update(raw).digest('hex').slice(0,8)}`;
     messages[key]=raw;
     migrations.push({file:relative,line:source.getLineAndCharacterOfPosition(item.getStart(source)).line+1,key,source:raw,kind:'whole-label-with-decorative-icons'});
     edits.push({start:item.getStart(source),end:item.end,text:`<LocalizedText messageKey=${JSON.stringify(key)} />${item.text.match(/\s*$/)?.[0]??''}`});
    }
   }
  }
  ts.forEachChild(node,visit);
 }
 visit(source);
 if(apply&&edits.length){let next=text;for(const edit of edits.sort((a,b)=>b.start-a.start))next=next.slice(0,edit.start)+edit.text+next.slice(edit.end);if(!/import\s*\{[^}]*\bLocalizedText\b/.test(text)){let from=path.relative(path.dirname(file),path.join(root,'i18n/LocalizedText')).split(path.sep).join('/');if(!from.startsWith('.'))from='./'+from;next=`import {LocalizedText} from ${JSON.stringify(from)};\n`+next;}fs.writeFileSync(file,next);}
}}
walk(root);
if(apply){fs.writeFileSync(new URL('../src/i18n/sourceMessages.ts',import.meta.url),`/** Explicit source-assigned static text keys. Missing locale entries remain English fallback. */\nexport const sourceMessages = ${JSON.stringify(messages,null,2)} as const;\n`);fs.writeFileSync(manifestURL,JSON.stringify(migrations,null,2)+'\n');}
console.log(JSON.stringify({mode:apply?'apply':'preview',sinks:migrations.length,files:new Set(migrations.map(item=>item.file)).size,labels:migrations.map(item=>item.source)}));
