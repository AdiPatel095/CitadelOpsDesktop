import ts from 'typescript';
import fs from 'node:fs';
import path from 'node:path';
const root = new URL('../src/', import.meta.url).pathname;
const entries = [];
const exclusions = [];
const visibleAttributes = /^(title|alt|placeholder|aria-label|aria-description|label|description|message|help|tooltip|emptyText|heading)$/;
function walk(dir) {
  for (const name of fs.readdirSync(dir).sort()) {
    const file = path.join(dir, name);
    if (fs.statSync(file).isDirectory()) { walk(file); continue; }
    if (!/\.tsx?$/.test(name)) continue;
    const source = ts.createSourceFile(file, fs.readFileSync(file, 'utf8'), ts.ScriptTarget.Latest, true);
    function visit(node) {
      if (ts.isJsxText(node) || ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node) || ts.isTemplateExpression(node)) {
        const text = ts.isTemplateExpression(node) ? node.getText(source) : node.text;
        if (text.trim() && /[A-Za-z]/.test(text)) {
          const parent = node.parent;
          let reason = null;
          if (ts.isImportDeclaration(parent) || ts.isExportDeclaration(parent) || ts.isLiteralTypeNode(parent)) reason = 'module path or type-only literal';
          else if (ts.isJsxAttribute(parent) && /^(className|id|key|href|src|type|role|data-|style)/.test(parent.name.getText(source))) reason = 'structural JSX attribute; audit if rendered as content';
          else if (ts.isCallExpression(parent) && /^console\./.test(parent.expression.getText(source))) reason = 'private developer console diagnostic';
          let category = 'requires-dataflow-review';
          let ancestor = parent;
          while (ancestor && !ts.isStatement(ancestor)) {
            if (ts.isJsxAttribute(ancestor) && visibleAttributes.test(ancestor.name.getText(source))) { category = 'visible-attribute'; break; }
            if (ts.isJsxExpression(ancestor)) category = 'rendered-expression';
            ancestor = ancestor.parent;
          }
          if (ts.isJsxText(node)) category = 'visible-jsx-text';
          if (ts.isPropertyAssignment(parent) && visibleAttributes.test(parent.name.getText(source).replaceAll("'",''))) category = 'display-message-property';
          if (ts.isCallExpression(parent) && /^(Notifications\.|alert$|confirm$|set[A-Za-z]*(Error|Message)$)/.test(parent.expression.getText(source))) category = 'notification-or-error';
          if (reason) { exclusions.push({file:path.relative(root,file),line:source.getLineAndCharacterOfPosition(node.getStart(source)).line+1,reason}); }
          else entries.push({file:path.relative(root,file),line:source.getLineAndCharacterOfPosition(node.getStart(source)).line+1,kind:ts.SyntaxKind[node.kind],category,text:text.trim(),route:category==='requires-dataflow-review'?'unresolved':'explicit-custom-message-key',classification:'unreviewed'});
        }
      }
      ts.forEachChild(node,visit);
    }
    visit(source);
  }
}
walk(root);
const categories = Object.fromEntries([...new Set(entries.map(e=>e.category))].map(key=>[key,entries.filter(e=>e.category===key).length]));
const report={schemaVersion:2,policy:'Conservative source inventory, not semantic coverage certification. Explicit sinks require message key assignment; unresolved candidates require dataflow review. Dynamic API strings need separate descriptor inventory. Technical literals omitted from entries but source references and exclusion reasons retained.',sourceFiles:new Set([...entries,...exclusions].map(e=>e.file)).size,counts:{total:entries.length+exclusions.length,unreviewed:entries.length,excluded:exclusions.length,categories},entries,exclusions};
fs.writeFileSync(new URL('../localization/source-inventory.json',import.meta.url),JSON.stringify(report)+'\n');
console.log(JSON.stringify({sourceFiles:report.sourceFiles,...report.counts}));
