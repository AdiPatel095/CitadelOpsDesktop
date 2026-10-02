import { readdirSync, readFileSync, statSync } from 'node:fs';
import { resolve, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

export const BUTTON_PATTERNS = Object.freeze(['row', 'card', 'tab', 'disclosure', 'nav', 'tile']);
export const EXEMPTIONS = Object.freeze([
  'components/ui/ (shared primitives)',
  'test files and mock/',
  'components/Header.tsx raw-button markers (CIT-69; preserved main-checkout edits)',
]);

function exempt(file) {
  return /(?:^|\/)components\/ui\//.test(file)
    || /(?:^|\/)(?:tests?|__tests__|mock)\//.test(file)
    || /\.(?:test|spec)\.tsx$/.test(file);
}

function unparen(expression) {
  while (expression && ts.isParenthesizedExpression(expression)) expression = expression.expression;
  return expression;
}

function attributeValue(attribute) {
  let value = attribute?.initializer;
  if (value && ts.isJsxExpression(value)) value = unparen(value.expression);
  return value;
}

function literal(attribute) {
  const value = attributeValue(attribute);
  return value && (ts.isStringLiteral(value) || ts.isNoSubstitutionTemplateLiteral(value)) ? value.text : undefined;
}

// Syntax checks only: the marker documents a pattern; its appearance and behavior
// still need the CIT-65 visual/accessibility review. Spreads cannot prove a marker
// or required prop, so those attributes must be written explicitly at the site.
export function checkSource(source, file = 'fixture.tsx') {
  file = file.split(sep).join('/');
  if (exempt(file)) return [];
  const ast = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const findings = [];
  function report(node, reason) {
    const { line } = ast.getLineAndCharacterOfPosition(node.getStart(ast));
    findings.push({ file, line: line + 1, reason });
  }
  for (const diagnostic of ast.parseDiagnostics) {
    const { line } = ast.getLineAndCharacterOfPosition(diagnostic.start ?? 0);
    findings.push({ file, line: line + 1, reason: `TSX parse error: ${ts.flattenDiagnosticMessageText(diagnostic.messageText, ' ')}` });
  }
  function visit(node) {
    if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
      const tag = node.tagName.getText(ast);
      const attributes = new Map(node.attributes.properties
        .filter(ts.isJsxAttribute)
        .map((attribute) => [attribute.name.getText(ast), attribute]));
      if (tag === 'button' && !/(?:^|\/)components\/Header\.tsx$/.test(file)) {
        const pattern = attributes.get('data-button-pattern');
        if (!BUTTON_PATTERNS.includes(literal(pattern))) {
          report(pattern ?? node, `<button> needs a literal data-button-pattern (${BUTTON_PATTERNS.join(', ')})`);
        }
      } else if (tag === 'Button') {
        if (literal(attributes.get('variant')) === 'danger' && !attributes.get('leftIcon')?.initializer) {
          report(node, '<Button variant="danger"> needs leftIcon=');
        }
        const iconOnly = attributes.get('iconOnly');
        if (iconOnly && attributeValue(iconOnly)?.kind !== ts.SyntaxKind.FalseKeyword && !attributes.get('aria-label')?.initializer) {
          report(node, '<Button iconOnly> needs aria-label=');
        }
      }
    }
    // Include strings in class helpers, object keys and template fragments, not
    // just className attributes. Comments and JSX text are not string tokens.
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)
      || ts.isTemplateHead(node) || ts.isTemplateMiddle(node) || ts.isTemplateTail(node)) {
      if (/(?:^|\s)m3-button(?:-[\w-]*)?(?=\s|$)/.test(node.text)) {
        report(node, 'm3-button classes are allowed only in components/ui/');
      }
    }
    ts.forEachChild(node, visit);
  }
  visit(ast);
  return findings.sort((a, b) => a.line - b.line || a.reason.localeCompare(b.reason));
}

export function checkRoot(root) {
  root = resolve(root);
  if (!statSync(root).isDirectory()) throw new Error(`root is not a directory: ${root}`);
  const findings = [];
  let files = 0;
  function walk(directory) {
    for (const entry of readdirSync(directory, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      const path = resolve(directory, entry.name);
      const file = relative(root, path).split(sep).join('/');
      if (entry.isDirectory()) {
        if (!entry.name.startsWith('.') && entry.name !== 'node_modules' && !exempt(`${file}/`)) walk(path);
      } else if (entry.isFile() && entry.name.endsWith('.tsx') && !exempt(file)) {
        files++;
        findings.push(...checkSource(readFileSync(path, 'utf8'), file));
      }
    }
  }
  walk(root);
  return { files, findings };
}

export function main(args = process.argv.slice(2)) {
  const reportMode = args.includes('--report');
  const roots = args.filter((arg) => arg !== '--report');
  if (roots.length !== 1 || roots[0].startsWith('-')) {
    console.error('Usage: node scripts/check-raw-buttons.mjs <root> [--report]');
    return 2;
  }
  try {
    const { files, findings } = checkRoot(roots[0]);
    for (const { file, line, reason } of findings) console.log(`${file}:${line} ${reason}`);
    console.log(`Raw-button ${reportMode ? 'report' : 'check'}: ${findings.length} violation(s) in ${files} TSX file(s).`);
    console.log(`Exemptions: ${EXEMPTIONS.join('; ')}.`);
    return reportMode || findings.length === 0 ? 0 : 1;
  } catch (error) {
    console.error(`Raw-button checker: ${error.message}`);
    return 2;
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) process.exitCode = main();
