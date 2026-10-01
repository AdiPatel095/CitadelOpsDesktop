import { readFileSync, readdirSync, lstatSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';
import postcss from 'postcss';
import ts from 'typescript';

// CIT-60 §4.3. These are migration targets, not runtime token definitions.
export const roles = Object.freeze({
  'display-lg': { size: 64, line: 68, weight: 700 },
  display: { size: 48, line: 52, weight: 700 },
  'display-sm': { size: 32, line: 40, weight: 700 },
  headline: { size: 24, line: 32, weight: 700 },
  title: { size: 20, line: 28, weight: 600 },
  'title-sm': { size: 16, line: 24, weight: 600 },
  'body-lg': { size: 16, line: 24, weight: 400 },
  body: { size: 14, line: 20, weight: 400 },
  'body-sm': { size: 13, line: 18, weight: 400 },
  caption: { size: 12, line: 16, weight: 400 },
});

export function mapWeight(value) {
  const named = { normal: 400, bold: 700 };
  const number = named[value] ?? (/^\d+$/.test(String(value)) ? Number(value) : NaN);
  if ([400, 500, 600, 700].includes(number)) return number;
  if (number === 520) return 500;
  if (number >= 620 && number <= 650) return 600;
  if (number >= 700 && number <= 1000) return 700;
  return null; // The plan does not specify lighter/intermediate weights.
}

export function pixels(value, remBase) {
  const match = String(value).match(/^(\d*\.?\d+)(px|rem)?$/);
  if (!match) return null;
  const number = Number(match[1]);
  if (!(number > 0)) return null;
  if (match[2] === 'rem') return remBase ? number * remBase : null;
  return number;
}

export function nearestRoles(size) {
  if (size < 12) return ['caption'];
  const distance = Math.min(...Object.values(roles).map(role => Math.abs(role.size - size)));
  return Object.keys(roles).filter(name => Math.abs(roles[name].size - size) === distance);
}

/** Pure transform. Keys address the ORIGINAL source; expected prevents stale decisions. */
export function transform(source, { file = 'input.tsx', decisions = {}, remBase } = {}) {
  const changes = [], unresolved = [], used = [], edits = [];
  function record(kind, offset, value, replacement, reason, extra = {}) {
    const key = `${file}:${offset}:${kind}`;
    const item = { key, kind, offset, before: value, ...extra };
    if (replacement === null) unresolved.push({ ...item, reason });
    else if (replacement !== value) changes.push({ ...item, after: replacement });
    return replacement;
  }
  function sizeAt(value, offset, css = false) {
    const key = `${file}:${offset}:size`;
    const decision = Object.hasOwn(decisions, key) ? decisions[key] : undefined;
    const size = pixels(value, remBase);
    let role;
    if (decision !== undefined) {
      used.push(key);
      if (!decision || typeof decision !== 'object' || decision.expected !== value || !Object.hasOwn(roles, decision.role)
        || (decision.compact !== undefined && typeof decision.compact !== 'boolean')
        || (decision.compact && decision.role !== 'headline')) {
        throw new Error(`${key}: stale or invalid role decision`);
      }
      role = decision.role;
    }
    if (size === null) return record('size', offset, value, null, 'dynamic/relative size requires manual review');
    if (size < 12) {
      if (role && role !== 'caption') throw new Error(`${key}: sub-12px text must map to caption`);
      role = 'caption';
    }
    if (!role) return record('size', offset, value, null, 'choose a role by element use', { candidates: nearestRoles(size), pixels: size });
    const target = decision?.compact ? 20 : roles[role].size;
    return record('size', offset, value, css ? `var(--font-size-${target})` : `text-${role}${decision?.compact ? '-compact' : ''}`, null, { role });
  }
  function weightAt(value, offset, css = false) {
    const weight = mapWeight(value);
    return record('weight', offset, value, weight === null ? null
      : css ? `var(--font-weight-${weight})` : `font-${{ 400: 'normal', 500: 'medium', 600: 'semibold', 700: 'bold' }[weight]}`,
    'weight is outside the plan mapping');
  }
  function classes(text, offset) {
    // Token-local replacement keeps variants, !important, whitespace and colors.
    return text.replace(/\S+/g, (token, index) => {
      const size = /text-\[((?:\d*\.)?\d+(?:px|rem))\](?:\/[^\s]+)?/.exec(token);
      const weight = /font-\[([^\]]+)\]/.exec(token);
      const match = size ?? weight;
      if (!match) {
        if (/(?:^|:)[!]?(?:text-(?:xs|sm|base|lg|xl|[2-9]xl))\//.test(token)) {
          record('size', offset + index, token, null, 'custom leading requires manual review');
          return token;
        }
        const legacy = /^(.*:)?(!?)(text-(xs|sm|base|lg|xl|[2-9]xl)|font-(thin|extralight|light|normal|medium|semibold|bold|extrabold|black))(!?)$/.exec(token);
        if (legacy) {
          const [, variant = '', before, , textName, weightName, after] = legacy;
          const value = textName ? `${{ xs: 12, sm: 14, base: 16, lg: 18, xl: 20, '2xl': 24, '3xl': 30, '4xl': 36, '5xl': 48, '6xl': 60, '7xl': 72, '8xl': 96, '9xl': 128 }[textName]}px`
            : String({ thin: 100, extralight: 200, light: 300, normal: 400, medium: 500, semibold: 600, bold: 700, extrabold: 800, black: 900 }[weightName]);
          // Allowed weight utilities already express the target and need no edit.
          if (['normal', 'medium', 'semibold', 'bold'].includes(weightName)) return token;
          const mapped = textName ? sizeAt(value, offset + index + variant.length + before.length) : weightAt(value, offset + index + variant.length + before.length);
          return mapped === null ? token : variant + before + mapped + after;
        }
        if (/text-\[[^\]]*(?:px|rem|em|vw|calc\()/.test(token)) record('size', offset + index, token, null, 'dynamic class requires manual review');
        return token;
      }
      const prefix = token.slice(0, match.index);
      if (!/^(?:.*:)?!?$/.test(prefix) || !['', '!'].includes(token.slice(match.index + match[0].length))) return token;
      // Custom leading overrides a role's paired line height; never silently retain it.
      if (size && match[0].includes('/')) {
        record('size', offset + index + match.index, match[1], null, 'custom leading requires manual review');
        return token;
      }
      const mapped = size ? sizeAt(match[1], offset + index + match.index) : weightAt(match[1], offset + index + match.index);
      return mapped === null ? token : prefix + mapped + token.slice(match.index + match[0].length);
    });
  }

  let output = source;
  if (file.endsWith('.css')) {
    const root = postcss.parse(source, { from: file });
    root.walkDecls(decl => {
      const prop = decl.prop.toLowerCase();
      if (!['font-size', 'font-weight', 'font'].includes(prop)) return;
      const offset = decl.source.start.offset;
      if (/^(?:var\(|inherit$|initial$|unset$|revert(?:-layer)?$)/.test(decl.value)) return;
      if (prop === 'font') {
        record('shorthand', offset, decl.value, null, 'font shorthand requires manual review');
        return;
      }
      if (decl.raws.value?.raw.includes('/*')) {
        record(prop === 'font-size' ? 'size' : 'weight', offset, decl.value, null, 'comment inside value requires manual review');
        return;
      }
      const mapped = prop === 'font-size' ? sizeAt(decl.value, offset, true) : weightAt(decl.value, offset, true);
      if (mapped !== null) decl.value = mapped;
    });
    output = root.toString();
  } else {
    const ast = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
    if (ast.parseDiagnostics.length) throw new Error(`${file}: invalid TSX syntax`);
    const visited = new Set();
    function edit(node, value) {
      edits.push({ start: node.getStart(ast), end: node.end, value });
    }
    function classExpression(node) {
      if (visited.has(node)) return;
      visited.add(node);
      if (ts.isStringLiteralLike(node)) {
        const start = node.getStart(ast), raw = source.slice(start + 1, node.end - 1);
        const result = classes(raw, start + 1);
        if (result !== raw) edit(node, source[start] + result + source[node.end - 1]);
      } else if (ts.isTemplateExpression(node)) {
        // Do not rewrite a literal fragment of a dynamically assembled class.
        record('class', node.getStart(ast), node.getText(ast), null, 'interpolated class template requires manual review');
      } else if (ts.isConditionalExpression(node)) {
        classExpression(node.whenTrue); classExpression(node.whenFalse);
      } else if (ts.isBinaryExpression(node)) {
        if ([ts.SyntaxKind.AmpersandAmpersandToken, ts.SyntaxKind.BarBarToken, ts.SyntaxKind.QuestionQuestionToken].includes(node.operatorToken.kind)) classExpression(node.right);
        else record('class', node.getStart(ast), node.getText(ast), null, 'assembled class requires manual review');
      } else if (ts.isCallExpression(node)) {
        if (['cn', 'clsx', 'classNames'].includes(node.expression.getText(ast))) node.arguments.forEach(classExpression);
        else record('class', node.getStart(ast), node.getText(ast), null, 'class helper requires manual review');
      } else if (ts.isArrayLiteralExpression(node)) node.elements.forEach(classExpression);
      else if (ts.isObjectLiteralExpression(node)) {
        const before = { edits: edits.length, changes: changes.length, unresolved: unresolved.length, used: used.length };
        const keys = new Set();
        let collision = false;
        for (const prop of node.properties) {
          if (!ts.isPropertyAssignment(prop) || !ts.isStringLiteral(prop.name)) {
            record('class', prop.getStart(ast), prop.getText(ast), null, 'computed/spread class key requires manual review');
            continue;
          }
          classExpression(prop.name);
          const replacement = edits.find(edit => edit.start === prop.name.getStart(ast));
          const key = replacement ? replacement.value.slice(1, -1) : prop.name.text;
          if (keys.has(key)) collision = true;
          keys.add(key);
        }
        if (collision) {
          edits.length = before.edits; changes.length = before.changes;
          unresolved.length = before.unresolved; used.length = before.used;
          record('class', node.getStart(ast), node.getText(ast), null, 'mapped object keys collide; preserve each condition manually');
        }
      } else if (ts.isParenthesizedExpression(node) || ts.isAsExpression(node) || ts.isSatisfiesExpression(node)) classExpression(node.expression);
      else record('class', node.getStart(ast), node.getText(ast), null, 'class reference requires manual review');
    }
    function inlineValue(node, kind) {
      const raw = ts.isStringLiteralLike(node) ? node.text : node.getText(ast);
      if (/^(?:var\(|inherit$|initial$|unset$)/.test(raw)) return;
      const mapped = kind === 'size' ? sizeAt(raw, node.getStart(ast), true) : weightAt(raw, node.getStart(ast), true);
      if (mapped !== null) edit(node, JSON.stringify(mapped));
    }
    function visit(node) {
      if (ts.isJsxAttribute(node)) {
        const name = node.name.getText(ast), init = node.initializer;
        const value = init && ts.isJsxExpression(init) ? init.expression : init;
        if (value && name === 'className') classExpression(value);
        if (value && ['fontSize', 'fontWeight'].includes(name)) inlineValue(value, name === 'fontSize' ? 'size' : 'weight');
        if (value && name === 'style' && ts.isObjectLiteralExpression(value)) {
          for (const prop of value.properties) {
            if (!ts.isPropertyAssignment(prop)) continue;
            const name = ts.isIdentifier(prop.name) || ts.isStringLiteral(prop.name) ? prop.name.text : '';
            if (['fontSize', 'fontWeight'].includes(name)) inlineValue(prop.initializer, name === 'fontSize' ? 'size' : 'weight');
          }
        }
      }
      ts.forEachChild(node, visit);
    }
    visit(ast);
    for (const edit of edits.sort((a, b) => b.start - a.start)) output = output.slice(0, edit.start) + edit.value + output.slice(edit.end);
  }
  return { output, changes, unresolved, used };
}

/** Dry run is the default. A write batch requires every finding to be resolved. */
export function run({ root, files, decisions = {}, remBase, write = false }) {
  root = path.resolve(root);
  if (remBase !== undefined && !(Number.isFinite(remBase) && remBase > 0)) throw new Error('rem base must be positive');
  const paths = new Set();
  function collect(relative) {
    const absolute = path.resolve(root, relative);
    if (absolute !== root && !absolute.startsWith(root + path.sep)) throw new Error('path must stay inside root');
    // Refuse symlinks at every segment, including symlinked parent directories.
    let current = root;
    for (const part of path.relative(root, absolute).split(path.sep).filter(Boolean)) {
      current = path.join(current, part);
      if (lstatSync(current).isSymbolicLink()) throw new Error('symlink targets are excluded');
    }
    if (lstatSync(absolute).isDirectory()) {
      for (const name of readdirSync(absolute).sort()) {
        if (['node_modules', '.git', 'dist', '.visual'].includes(name)) continue;
        collect(path.relative(root, path.join(absolute, name)));
      }
    } else if (/\.(css|tsx)$/.test(absolute)) {
      if (/\/(?:styles\/)?tokens\.css$/.test(absolute)) return; // Raw token definitions are not migration inputs.
      paths.add(path.relative(root, absolute).split(path.sep).join('/'));
    }
  }
  files.forEach(collect);
  const results = [...paths].sort().map(file => ({ file, ...transform(readFileSync(path.join(root, file), 'utf8'), { file, decisions, remBase }) }));
  const used = new Set(results.flatMap(result => result.used));
  for (const key of Object.keys(decisions)) if (!used.has(key)) throw new Error(`${key}: unused/stale decision`);
  const unresolved = results.flatMap(result => result.unresolved);
  if (write && unresolved.length) throw new Error(`Refusing writes: ${unresolved.length} unresolved findings`);
  if (write) for (const result of results) if (result.changes.length) writeFileSync(path.join(root, result.file), result.output);
  return { mode: write ? 'write' : 'dry-run', files: results.map(({ output, used, ...result }) => result), unresolved: unresolved.length };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const { values, positionals } = parseArgs({ allowPositionals: true, options: {
      root: { type: 'string', default: '.' }, decisions: { type: 'string' },
      'rem-base': { type: 'string' }, write: { type: 'boolean', default: false },
    } });
    if (!positionals.length) throw new Error('Usage: node scripts/typography/type-scale.mjs [--root DIR] [--decisions FILE] [--rem-base 16] [--write] src');
    const report = run({ root: values.root, files: positionals,
      decisions: values.decisions ? JSON.parse(readFileSync(values.decisions, 'utf8')) : {},
      remBase: values['rem-base'] === undefined ? undefined : Number(values['rem-base']), write: values.write });
    console.log(JSON.stringify(report, null, 2));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
