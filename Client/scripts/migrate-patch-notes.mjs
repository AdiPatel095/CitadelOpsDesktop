import ts from 'typescript';
import fs from 'node:fs';
import {createHash} from 'node:crypto';
const file=new URL('../src/config/PatchNotes.ts',import.meta.url);
const text=fs.readFileSync(file,'utf8');
if(text.includes('textKey:'))throw new Error('Patch-note source migration already applied');
const ast=ts.createSourceFile(file.pathname,text,ts.ScriptTarget.Latest,true);
const sourceFile=new URL('../src/i18n/sourceMessages.ts',import.meta.url);
const sourceText=fs.readFileSync(sourceFile,'utf8');
const catalog=JSON.parse(sourceText.match(/export const sourceMessages = ([\s\S]+) as const;/)[1]);
const edits=[],manifest=[],literalText={};
function visit(node) {
 if(ts.isPropertyAssignment(node)&&['text','subtitle'].includes(node.name.getText(ast))&&ts.isStringLiteral(node.initializer)) {
  let owner=node.parent;
  while(owner && !(ts.isObjectLiteralExpression(owner)&&owner.properties.some(property=>ts.isPropertyAssignment(property)&&property.name.getText(ast)==='version')))owner=owner.parent;
  if(!owner)return;
  const version=owner.properties.find(property=>ts.isPropertyAssignment(property)&&property.name.getText(ast)==='version').initializer.text;
  const field=node.name.getText(ast),value=node.initializer.text;
  const key=`patchNotes.release.${version}.${field}.${createHash('sha256').update(value).digest('hex').slice(0,12)}`;
  if(/[{}]/.test(value)) {
   literalText[key]=value;
   catalog[key]=value.replaceAll("'","''").replaceAll('{',"'{'").replaceAll('}',"'}'");
  } else catalog[key]=value;
  edits.push({at:node.getStart(ast),value:`${field}Key: ${JSON.stringify(key)},\n        `});
  manifest.push({version,field,key,source:value});
 }
 ts.forEachChild(node,visit);
}
visit(ast);
let next=text;
for(const edit of edits.sort((a,b)=>b.at-a.at))next=next.slice(0,edit.at)+edit.value+next.slice(edit.at);
next="import type {MessageKey} from '../i18n/messages';\n"+next;
next=next.replace('  text: string;','  text: string;\n  textKey: MessageKey;').replace('  subtitle?: string;','  subtitle?: string;\n  subtitleKey?: MessageKey;');
fs.writeFileSync(file,next);
fs.writeFileSync(sourceFile,`/** Explicit source-assigned static text keys. Missing locale entries remain English fallback. */\nexport const sourceMessages = ${JSON.stringify(catalog,null,2)} as const;\n`);
fs.writeFileSync(new URL('../localization/patch-note-migrations.json',import.meta.url),JSON.stringify(manifest,null,2)+'\n');
fs.writeFileSync(new URL('../src/i18n/patchNoteLiteralText.ts',import.meta.url),`/** Literal technical syntax in release prose; ICU source quotes these braces. */\nexport const patchNoteLiteralText:Readonly<Record<string,string>> = ${JSON.stringify(literalText,null,2)};\n`);
console.log(JSON.stringify({keys:manifest.length}));
