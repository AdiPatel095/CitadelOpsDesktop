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
    if (path.relative(root,file).startsWith('i18n/')) continue; // Catalog/formatter coverage has its own strict tests, not unmigrated source.
    const source = ts.createSourceFile(file, fs.readFileSync(file, 'utf8'), ts.ScriptTarget.Latest, true);
    function visit(node) {
      if (ts.isJsxText(node) || ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node) || ts.isTemplateExpression(node)) {
        const text = ts.isTemplateExpression(node) ? node.getText(source) : node.text;
        if (text.trim() && /[A-Za-z]/.test(text)) {
          const parent = node.parent;
          let reason = null;
          if (ts.isJsxAttribute(parent) && parent.name.getText(source) === 'messageKey' && ts.isJsxSelfClosingElement(parent.parent?.parent) && ['LocalizedText','LocalizedRichText'].includes(parent.parent.parent.tagName.getText(source))) reason = 'explicit typed LocalizedText key; source assignment in static-migrations.json';
          else if (ts.isCallExpression(parent) && /^(t|message|localizeStatic)$/.test(parent.expression.getText(source)) && node === parent.arguments[0]) reason = 'explicit typed localization key reference';
          else if (ts.isImportDeclaration(parent) || ts.isExportDeclaration(parent) || ts.isLiteralTypeNode(parent)) reason = 'module path or type-only literal';
          else if (ts.isJsxAttribute(parent) && /^(className|id|key|href|src|type|role|data-|style)/.test(parent.name.getText(source))) reason = 'structural JSX attribute; audit if rendered as content';
          else if (ts.isCallExpression(parent) && /^console\./.test(parent.expression.getText(source))) reason = 'private developer console diagnostic';
          // Exclude only literals whose syntactic use is a selector or formatting instruction.
          // State values and returned messages remain unresolved because they may reach the UI.
          if (!reason && ts.isElementAccessExpression(parent) && parent.argumentExpression === node) reason = 'property selector, not its displayed value';
          if (!reason && ts.isCaseClause(parent) && parent.expression === node) reason = 'switch discriminator, not case output';
          if (!reason && ts.isBinaryExpression(parent) && [ts.SyntaxKind.EqualsEqualsToken,ts.SyntaxKind.EqualsEqualsEqualsToken,ts.SyntaxKind.ExclamationEqualsToken,ts.SyntaxKind.ExclamationEqualsEqualsToken].includes(parent.operatorToken.kind)) reason = 'comparison discriminator, not comparison output';
          if (!reason && ts.isCallExpression(parent) && parent.expression.kind === ts.SyntaxKind.ImportKeyword) reason = 'dynamic module path';
          if (!reason && ts.isCallExpression(parent) && node === parent.arguments[0] && /(?:^|\.)(querySelector|querySelectorAll|getElementById|getElementsByClassName|addEventListener|removeEventListener|getItem|setItem|removeItem)$/.test(parent.expression.getText(source))) reason = 'DOM event/storage selector; value remains separately inventoried';
          if (!reason && ts.isCallExpression(parent) && node === parent.arguments[0] && /^(runtimeFetch|fetch|configurationSection|asConfigurationSection)$/.test(parent.expression.getText(source))) reason = 'request path or configuration selector; response content remains inventoried';
          let context = parent;
          while (!reason && context && !ts.isStatement(context)) {
            if (ts.isJsxAttribute(context) && /^(className|class|style|id|key|href|src|role|data-[\w-]+)$/.test(context.name.getText(source))) reason = 'structural JSX expression; not rendered text';
            if ((ts.isNewExpression(context) || ts.isCallExpression(context)) && /^(new )?Intl\.(NumberFormat|DateTimeFormat|RelativeTimeFormat|PluralRules|ListFormat|DisplayNames)$/.test(context.expression.getText(source))) reason = 'Intl formatter option or locale identifier';
            if (ts.isCallExpression(context) && /^(clsx|classnames|cn|twMerge)$/.test(context.expression.getText(source))) reason = 'CSS class composition';
            context = context.parent;
          }
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
