// The Client owner runs this explicit copy after validating the source packs.
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFileSync} from 'node:child_process';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const [target,parserPath]=process.argv.slice(2);
if (!target || !parserPath) throw new Error('Usage: node sync-client.mjs /path/to/Client/src/i18n/server /path/to/icu-messageformat-parser/index.js');
if (!path.resolve(target).endsWith(path.join('Client','src','i18n','server'))) throw new Error('Target must be the explicit Client/src/i18n/server directory');
const report=JSON.parse(execFileSync(process.execPath,[path.join(root,'scripts/validate-locales.mjs'),parserPath],{encoding:'utf8'}));
fs.mkdirSync(target,{recursive:true});
const files={'en.json':path.join(root,'en.json')};
if(fs.existsSync(path.join(root,'feature-names.json'))) files['feature-names.json']=path.join(root,'feature-names.json');
for(const name of fs.readdirSync(path.join(root,'locales')).filter(name=>name.endsWith('.json')).sort())files[name]=path.join(root,'locales',name);
for(const [name,source] of Object.entries(files)){
 const bytes=fs.readFileSync(source);
 fs.writeFileSync(path.join(target,name),bytes);
}
console.log(JSON.stringify({target:path.resolve(target),files:Object.keys(files).length,validated:report.validated}));
