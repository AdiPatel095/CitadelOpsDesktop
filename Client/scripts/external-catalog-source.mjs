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
  const coverage=JSON.parse(read('coverage.json'));
  if(coverage.sourceHashes?.['en.json']!==hash(bytes) || coverage.sourceHashes?.['provenance.json']!==hash(provenanceBytes) || !provenance.sourceRevision || !provenance.entries || !Object.keys(provenance.entries).length)throw new Error(`${family}: missing or stale synchronization provenance`);
 } else throw new Error(`Unknown catalog family: ${family}`);
 return source;
}
/** Integration coverage must use the server source included in this checkout. */
export function assertRuntimeCatalogMatch(clientDirectory,runtimeSourcePath) {
 if(!fs.existsSync(runtimeSourcePath))throw new Error('server: integrated runtime source catalog is missing');
 const runtimeBytes=fs.readFileSync(runtimeSourcePath);
 const runtime=JSON.parse(runtimeBytes);
 if(!runtime||typeof runtime!=='object'||Array.isArray(runtime)||!Object.keys(runtime).length||Object.values(runtime).some(value=>typeof value!=='string'||!value.trim()))throw new Error('server: integrated runtime source catalog is invalid');
 const clientPath=path.join(clientDirectory,'en.json');
 if(!fs.existsSync(clientPath)||hash(fs.readFileSync(clientPath))!==hash(runtimeBytes))throw new Error('server: client catalog does not match integrated runtime source');
}
