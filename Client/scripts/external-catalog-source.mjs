import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
const hash=value=>createHash('sha256').update(value).digest('hex');
/** Missing catalog families are failures, never empty successful coverage baselines. */
export function readExternalCatalogSource(directory,family) {
 const read=name=>{const file=path.join(directory,name);if(!fs.existsSync(file))throw new Error(`${family}: missing ${name}`);return fs.readFileSync(file);};
 const bytes=read('en.json');const source=JSON.parse(bytes);
 if(!source || typeof source!=='object' || Array.isArray(source) || !Object.keys(source).length || Object.values(source).some(value=>typeof value!=='string'||!value.trim()))throw new Error(`${family}: empty or invalid source catalog`);
 const provenanceBytes=read('provenance.json');const provenance=JSON.parse(provenanceBytes);
 if(family==='backend') {
  if(provenance.source?.keyCount!==Object.keys(source).length || provenance.source?.sha256!==hash(JSON.stringify(source)) || !provenance.source?.revision || !provenance.packs || !Object.keys(provenance.packs).length)throw new Error(`${family}: missing or stale source provenance`);
 } else if(family==='server') {
  if(!provenance.sourceRevision || !provenance.entries || !Object.keys(provenance.entries).length)throw new Error(`${family}: missing or stale synchronization provenance`);
 } else throw new Error(`Unknown catalog family: ${family}`);
 return source;
}
/** Integration coverage must use the server source included in this checkout. */
export function assertServerCatalogSynchronized(clientDirectory,serverRoot) {
 const runtimeSourcePath=path.join(serverRoot,'en.json');
 if(!fs.existsSync(runtimeSourcePath))throw new Error('server: integrated runtime source catalog is missing');
 const runtimeBytes=fs.readFileSync(runtimeSourcePath);
 const runtime=JSON.parse(runtimeBytes);
 if(!runtime||typeof runtime!=='object'||Array.isArray(runtime)||!Object.keys(runtime).length||Object.values(runtime).some(value=>typeof value!=='string'||!value.trim()))throw new Error('server: integrated runtime source catalog is invalid');
 const sources=new Map([['en.json',runtimeSourcePath]]);
 const glossaryPath=path.join(serverRoot,'feature-names.json');
 if(fs.existsSync(glossaryPath)) sources.set('feature-names.json',glossaryPath);
 const localesDirectory=path.join(serverRoot,'locales');
 for(const name of fs.readdirSync(localesDirectory).filter(name=>name.endsWith('.json')).sort()) sources.set(name,path.join(localesDirectory,name));
 if(!sources.has('provenance.json')) throw new Error('server: integrated runtime provenance is missing');
 if(!fs.existsSync(clientDirectory)) throw new Error('server: client catalog directory is missing');
 const clientFiles=fs.readdirSync(clientDirectory,{recursive:true}).filter(name=>name.endsWith('.json'));
 for(const name of clientFiles) if(!sources.has(name)) throw new Error(`server: extra client catalog file ${name}`);
 for(const [name,sourcePath] of sources) {
  const clientPath=path.join(clientDirectory,name);
  if(!fs.existsSync(clientPath)) throw new Error(`server: missing client catalog file ${name}`);
  if(!fs.readFileSync(clientPath).equals(fs.readFileSync(sourcePath))) throw new Error(`server: client catalog ${name} does not match integrated runtime source`);
 }
}
