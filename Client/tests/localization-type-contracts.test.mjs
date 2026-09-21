import assert from 'node:assert/strict';
import test from 'node:test';
import ts from 'typescript';
import {fileURLToPath} from 'node:url';
test('localized message modules and equipment action callsites satisfy app type contracts',()=>{
 const root=fileURLToPath(new URL('..',import.meta.url));
 const config=ts.readConfigFile(`${root}/tsconfig.app.json`,ts.sys.readFile);
 assert.equal(config.error,undefined);
 const parsed=ts.parseJsonConfigFileContent(config.config,ts.sys,root);
 const program=ts.createProgram(parsed.fileNames,{...parsed.options,noEmit:true});
 const diagnostics=ts.getPreEmitDiagnostics(program).filter(item=>item.file && (/\/src\/i18n\//.test(item.file.fileName)||item.file.fileName.endsWith('/src/equipment/components/EquipmentView.tsx')));
 assert.equal(diagnostics.length,0,ts.formatDiagnosticsWithColorAndContext(diagnostics,{getCanonicalFileName:name=>name,getCurrentDirectory:()=>root,getNewLine:()=> '\n'}));
});
