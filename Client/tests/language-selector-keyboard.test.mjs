import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import ts from 'typescript';

test('searchable Select autofocuses search and restores trigger after Escape',async()=>{
 const stateUpdates=[],effects=[];let hook=0,ref=0,searchFocus=0,triggerFocus=0;
 const harness={
  React:{createElement:(type,props,...children)=>({type,props:{...props,children}})},
  useState:initial=>{const index=hook++;return [index===0?true:initial,value=>stateUpdates.push({index,value})];},
  useEffect:effect=>effects.push(effect),useId:()=> 'menu',useMemo:fn=>fn(),
  useRef:()=>({current:ref++===0?{querySelector:()=>({focus:()=>triggerFocus++})}:{focus:()=>searchFocus++}}),
  createPortal:content=>content,ChevronDown:'icon',CheckSquare:'icon',Search:'icon',
 };
 const previousWindow=globalThis.window,previousDocument=globalThis.document;
 globalThis.window={requestAnimationFrame:callback=>{callback();return 1;},cancelAnimationFrame(){},addEventListener(){},removeEventListener(){}};
 globalThis.document={body:{},addEventListener(){},removeEventListener(){}};
 globalThis.__selectHarness=harness;
 const source=fs.readFileSync(new URL('../src/components/ui/Select.tsx',import.meta.url),'utf8').replace(/^import .*?;\n/gm,'');
 const code=ts.transpileModule(`const {${Object.keys(harness).join(',')}}=globalThis.__selectHarness;\n${source}`,{compilerOptions:{module:ts.ModuleKind.ES2022,target:ts.ScriptTarget.ES2022,jsx:ts.JsxEmit.React}}).outputText;
 try {
  const {Select}=await import('data:text/javascript;base64,'+Buffer.from(code).toString('base64'));
  const tree=Select({value:'auto',options:[{value:'auto',label:'Automatic'}],onChange(){},searchable:true,closeOnScroll:false});
  const find=node=>node?.type==='input'?node:Array.isArray(node)?node.map(find).find(Boolean):node?.props?.children?.map(find).find(Boolean);
  const input=find(tree);assert.ok(input);assert.equal(input.props['aria-label'],'Filter options');
  const cleanups=effects.map(run=>run());assert.equal(searchFocus,1);
  let prevented=0,stopped=0;input.props.onKeyDown({key:'Escape',preventDefault:()=>prevented++,stopPropagation:()=>stopped++});
  assert.equal(triggerFocus,1);assert.equal(prevented,1);assert.equal(stopped,1);assert.deepEqual(stateUpdates.slice(-2),[{index:0,value:false},{index:1,value:''}]);
  cleanups.forEach(cleanup=>cleanup?.());
 } finally {globalThis.window=previousWindow;globalThis.document=previousDocument;delete globalThis.__selectHarness;}
});
