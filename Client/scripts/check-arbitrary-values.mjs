import { readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { parseArgs } from 'node:util';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';

const inlineProperty = /^(?:color|background|backgroundColor|border|borderTop|borderRight|borderBottom|borderLeft|borderBlock|borderInline|outline|.*Color|fontSize|fontWeight|border(?:TopLeft|TopRight|BottomLeft|BottomRight|StartStart|StartEnd|EndStart|EndEnd)?Radius|boxShadow|textShadow)$/;
export function arbitraryValueCount(text) {
  const tree = ts.createSourceFile('source.tsx', text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  let arbitrary = 0; let inline = 0;
  const definitions = new Map();
  function collect(node) {
    if (ts.isVariableDeclaration(node) && ts.isIdentifier(node.name) && node.initializer) definitions.set(node.name.text, node.initializer);
    ts.forEachChild(node, collect);
  }
  collect(tree);
  function style(node, seen = new Set()) {
    if (!node || seen.has(node)) return;
    seen.add(node);
    if (ts.isIdentifier(node)) { style(definitions.get(node.text), seen); return; }
    if (ts.isObjectLiteralExpression(node)) {
      for (const p of node.properties) {
        if (ts.isSpreadAssignment(p)) { style(p.expression, seen); continue; }
        if (!ts.isPropertyAssignment(p) || !inlineProperty.test(p.name.getText(tree).replace(/^['"]|['"]$/g, ''))) continue;
        if (!ts.isStringLiteralLike(p.initializer) || !/^var\(--[\w-]+\)$/.test(p.initializer.text.trim())) inline++;
      }
    } else if (ts.isConditionalExpression(node)) { style(node.whenTrue, seen); style(node.whenFalse, seen); }
    else if (ts.isParenthesizedExpression(node) || ts.isAsExpression(node) || ts.isSatisfiesExpression(node)) style(node.expression, seen);
  }
  function visit(node) {
    if (ts.isStringLiteralLike(node) || ts.isTemplateHead(node) || ts.isTemplateMiddle(node) || ts.isTemplateTail(node)) {
      arbitrary += (node.text.match(/[\w:!/-]+-\[[^\]\r\n]+\]|(?:[\w:!/-]+:)?\[[\w-]+:[^\]\r\n]+\]/g) ?? []).length;
    }
    if (ts.isJsxAttribute(node) && node.name.getText(tree) === 'style' && node.initializer && ts.isJsxExpression(node.initializer)) style(node.initializer.expression);
    ts.forEachChild(node, visit);
  }
  visit(tree);
  return { arbitrary, inline, total: arbitrary + inline };
}
export function ratchetFailures(baseline, counts) {
  return Object.entries(counts).filter(([file, count]) => count.total > (baseline[file]?.total ?? 0)).map(([file, count]) => `${file}: ${count.total} > ${baseline[file]?.total ?? 0}`);
}
function files(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => e.isDirectory() ? files(join(dir, e.name)) : e.name.endsWith('.tsx') ? [join(dir, e.name)] : []);
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const { values } = parseArgs({ options: { 'write-baseline': { type: 'boolean' } } });
  const counts = Object.fromEntries(files('src').sort().map((f) => [f, arbitraryValueCount(readFileSync(f, 'utf8'))]));
  if (values['write-baseline']) writeFileSync('arbitrary-values.baseline.json', JSON.stringify(counts, null, 2) + '\n');
  else {
    const failures = ratchetFailures(JSON.parse(readFileSync('arbitrary-values.baseline.json', 'utf8')), counts);
    if (failures.length) { console.error(failures.join('\n')); process.exit(1); }
  }
  console.log(`Arbitrary-value ratchet passed: ${Object.keys(counts).length} files, ${Object.values(counts).reduce((sum, c) => sum + c.total, 0)} values.`);
}
