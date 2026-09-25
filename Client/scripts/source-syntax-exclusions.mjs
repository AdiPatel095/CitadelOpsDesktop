import ts from 'typescript';
const svgTags=new Set(['svg','path','rect','circle','ellipse','line','polyline','polygon','g','defs','use','mask','clipPath','linearGradient','radialGradient','stop','symbol','marker','pattern','image','filter']);
const svgRenderingAttributes=new Set(['fill','stroke','strokeLinecap','strokeLinejoin','fillRule','clipRule','d','viewBox','transform','strokeDasharray','vectorEffect','preserveAspectRatio','xmlns']);
/** Narrow source-structural exclusions. Visible SVG title/desc/ARIA remain candidates. */
export function createSyntaxExclusionReviewer(source,knownKeys){
 const messageTypes=new Set(),stateFunctions=new Set();
 for(const statement of source.statements){
  if(!ts.isImportDeclaration(statement)||!ts.isStringLiteral(statement.moduleSpecifier))continue;
  const module=statement.moduleSpecifier.text;
  const bindings=statement.importClause?.namedBindings;
  if(!bindings||!ts.isNamedImports(bindings))continue;
  for(const item of bindings.elements){
   const imported=item.propertyName?.text??item.name.text;
   if(/(?:^|\/)i18n\/messages(?:\.ts)?$/.test(module)&&imported==='MessageKey')messageTypes.add(item.name.text);
   if(module==='react'&&imported==='useState')stateFunctions.add(item.name.text);
  }
 }
 return node=>{
  if(!ts.isStringLiteral(node)&&!ts.isNoSubstitutionTemplateLiteral(node))return null;
  let attribute=node.parent;
  if(ts.isJsxExpression(attribute)&&attribute.expression===node)attribute=attribute.parent;
  if(ts.isJsxAttribute(attribute)&&svgRenderingAttributes.has(attribute.name.getText(source))){
   const element=attribute.parent.parent;
   if((ts.isJsxOpeningElement(element)||ts.isJsxSelfClosingElement(element))&&svgTags.has(element.tagName.getText(source))){
    let ancestor=element;
    while(ancestor){
     if((ts.isJsxElement(ancestor)&&ancestor.openingElement.tagName.getText(source)==='svg')||((ts.isJsxOpeningElement(ancestor)||ts.isJsxSelfClosingElement(ancestor))&&ancestor.tagName.getText(source)==='svg'))return 'intrinsic SVG rendering attribute; accessible title/desc/ARIA excluded from this rule';
     ancestor=ancestor.parent;
    }
   }
  }
  const call=node.parent;
  if(ts.isCallExpression(call)&&ts.isIdentifier(call.expression)&&stateFunctions.has(call.expression.text)&&call.arguments[0]===node&&call.typeArguments?.length===1){
   const type=call.typeArguments[0];
   if(ts.isTypeReferenceNode(type)&&ts.isIdentifier(type.typeName)&&messageTypes.has(type.typeName.text)&&knownKeys.has(node.text))return 'known catalog key in explicitly typed MessageKey state; arbitrary key objects remain inventoried';
  }
  return null;
 };
}
