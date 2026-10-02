import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import postcss from 'postcss';
import ts from 'typescript';

const sizes = '12|13|14|16|20|24|32|48|64';
const weights = '400|500|600|700';
const inherited = /^(inherit|initial|unset|revert|revert-layer)$/;
const sizeToken = new RegExp(`^var\\(--font-size-(${sizes})\\)$`);
const weightToken = new RegExp(`^var\\(--font-weight-(${weights})\\)$`);
const namedRoles = /^(display-lg|display|display-sm|headline(?:-compact)?|title(?:-sm)?|body(?:-lg|-sm)?|caption|eyebrow)$/;

// Zero baseline: unlike the general arbitrary-value ratchet, typography has no allowance.
export function typographyFailures(source, file) {
  const failures = [];
  const fail = (line, detail) => failures.push(`${file}:${line}: ${detail}`);
  if (file.endsWith('.css')) {
    const root = postcss.parse(source, { from: file });
    root.walkDecls(d => {
      if (d.parent.type === 'atrule' && d.parent.name === 'font-face') return; // Font ranges describe files, not rendered weights.
      const line = d.source.start.line;
      if (d.prop === 'font-size' && !sizeToken.test(d.value) && !inherited.test(d.value)
        && !['var(--picker-card-name-size)', 'var(--liquid-pill-selector-font-size)'].includes(d.value)) fail(line, `unscaled font-size ${d.value}`);
      if (d.prop === 'font-weight' && !weightToken.test(d.value) && !inherited.test(d.value)) fail(line, `unscaled font-weight ${d.value}`);
      if (d.prop === 'font' && !inherited.test(d.value)) fail(line, `font shorthand bypasses scale: ${d.value}`);
      if (d.prop === 'text-transform' && !['none', 'var(--eyebrow-case)'].includes(d.value)) fail(line, 'uppercase is reserved for eyebrows');
      if (d.prop === 'letter-spacing' && !['0','var(--tracking-display)','var(--tracking-headline)','var(--tracking-eyebrow)'].includes(d.value)) fail(line, 'tracking must follow the role and locale');
    });
  } else {
    const tree = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
    const line = node => tree.getLineAndCharacterOfPosition(node.getStart(tree)).line + 1;
    function visit(node) {
      if (ts.isStringLiteralLike(node) || ts.isTemplateHead(node) || ts.isTemplateMiddle(node) || ts.isTemplateTail(node)) {
        for (const match of node.text.matchAll(/(?:^|[\s:!])text-(\[[^\]]+\]|xs|sm|base|lg|xl|[2-9]xl|display(?:-lg|-sm)?|headline(?:-compact)?|title(?:-sm)?|body(?:-lg|-sm)?|caption|eyebrow)(?:\/[^\s"'`]+)?/g)) {
          const arbitrarySize = match[1].startsWith('[') && !/^\[(?:#|(?:var\(--(?:md-sys-color|color|text)-))/.test(match[1]);
          if (arbitrarySize || (!match[1].startsWith('[') && !namedRoles.test(match[1])) || match[0].includes('/')) fail(line(node), `unscaled text utility ${match[0].trim()}`);
        }
        if (/(?:^|[\s:!])font-(?:\[[^\]]+\]|(?:thin|extralight|light|extrabold|black)\b)/.test(node.text)) fail(line(node), 'weight outside the four allowed utilities');
        if (/(?:^|[\s:!])(?:uppercase|tracking-(?!normal)[\w[.-]+)/.test(node.text)) fail(line(node), 'casing/tracking must use text-eyebrow');
      }
      if (ts.isPropertyAssignment(node) && ['fontSize','fontWeight'].includes(node.name.getText(tree).replace(/['"]/g,''))) {
        const name = node.name.getText(tree).replace(/['"]/g,'');
        if (!ts.isStringLiteralLike(node.initializer) || !(name === 'fontSize' ? sizeToken : weightToken).test(node.initializer.text)) fail(line(node), `unscaled inline ${name}`);
      }
      if (ts.isJsxAttribute(node) && ['fontSize','fontWeight'].includes(node.name.getText(tree))) {
        const value = node.initializer && ts.isJsxExpression(node.initializer) ? node.initializer.expression : node.initializer;
        if (!value || !ts.isStringLiteralLike(value) || !(node.name.getText(tree) === 'fontSize' ? sizeToken : weightToken).test(value.text)) fail(line(node), `unscaled SVG ${node.name.getText(tree)}`);
      }
      ts.forEachChild(node,visit);
    }
    visit(tree);
  }
  return failures;
}

function files(dir) {
  return readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?files(join(dir,e.name)):/\.(css|tsx|ts)$/.test(e.name)?[join(dir,e.name)]:[]);
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const targets = files('src').filter(f=>!f.endsWith('/tokens.css'));
  const failures = targets.flatMap(f=>typographyFailures(readFileSync(f,'utf8'),f));
  if(failures.length) { console.error(failures.join('\n')); process.exitCode=1; }
  else console.log(`Typography zero ratchet passed: ${targets.length} files, 9 sizes, 4 weights.`);
}
