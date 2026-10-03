import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import vm from 'node:vm';
import ts from 'typescript';
import * as react from './support/miniReact.mjs';

function loadModule(path, imports, globals = {}) {
  const module = { exports: {} };
  const source = readFileSync(new URL(path, import.meta.url), 'utf8');
  const code = ts.transpileModule(source, { compilerOptions: {
    module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022,
    jsx: ts.JsxEmit.React, esModuleInterop: true,
  } }).outputText;
  vm.runInNewContext(code, {
    module, exports: module.exports, crypto: { randomUUID: () => 'new-slot' }, ...globals,
    require: id => {
      assert.ok(Object.hasOwn(imports, id), `Unmocked boundary: ${id}`);
      return imports[id];
    },
  });
  return module.exports;
}

const schedulerTypes = loadModule('../src/settings/SchedulerTypes.ts', {});
const metadata = { troops: {}, getTroop: () => undefined, getTool: () => undefined };

function harness(value) {
  const listeners = new Map();
  const document = { body: { style: { userSelect: 'text' } } };
  const window = {
    addEventListener: (name, listener) => listeners.set(name, listener),
    removeEventListener: (name, listener) => {
      if (listeners.get(name) === listener) listeners.delete(name);
    },
    setTimeout: callback => callback(),
  };
  const imports = {
    react,
    'lucide-react': Object.fromEntries(['CalendarDays', 'Clock', 'Plus', 'Search', 'Trash2', 'Wand2'].map(name => [name, name])),
    '../../i18n/useLocale': { useLocale: () => ({ t: key => key }) },
    '../../i18n/LocalizedText': { LocalizedText: 'LocalizedText' },
    '../../components/ui': Object.fromEntries(['Badge', 'Button', 'Input', 'Select', 'SettingsModal', 'Switch'].map(name => [name, name])),
    '../../components/TroopPicker': { showTroopPicker: () => {} },
    '../../components/ToolPicker': { showToolPicker: () => {} },
    '../../components/UnitImage': { default: 'UnitImage' },
    '../../components/ToolImage': { default: 'ToolImage' },
    '../../context/useMetadata': { useMetadata: () => metadata },
    '../UnitUpgradeFamily': {},
    '../SchedulerTypes': schedulerTypes,
  };
  const { WeeklyScheduler } = loadModule('../src/settings/components/WeeklyScheduler.tsx', imports, { document, window });
  const changes = [];
  const onChange = next => changes.push(next);
  const component = react.mount(WeeklyScheduler);
  component.render({ value, onChange });
  const find = (node, attribute) => {
    if (Array.isArray(node)) return node.map(child => find(child, attribute)).find(Boolean);
    if (node?.props && Object.hasOwn(node.props, attribute)) return node;
    return find(node?.props?.children ?? [], attribute);
  };
  const pointer = (clientY = 22) => ({
    button: 0, clientX: 10, clientY, preventDefault() {}, stopPropagation() {},
    currentTarget: {
      getBoundingClientRect: () => ({ top: 0, height: 36 }),
      closest: () => ({ getBoundingClientRect: () => ({ left: 0, width: 128 }) }),
    },
  });
  return { component, document, listeners, changes, find, pointer,
    rerender: next => component.render({ value: next, onChange }) };
}

const schedule = {
  enabled: true, timeZone: 'UTC', slots: [
    { id: 'slot', day: 1, startMinute: 60, endMinute: 180 },
  ],
};

test('slot dragging keeps selection disabled across schedule updates, then restores it on release', () => {
  const h = harness(schedule);
  try {
    assert.equal(h.document.body.style.userSelect, 'text', 'mount must preserve the existing style');
    h.find(h.component.value, 'data-schedule-slot').props.onPointerDown(h.pointer());
    h.component.settle();
    assert.equal(h.document.body.style.userSelect, 'none');
    h.listeners.get('pointermove')(h.pointer(40));
    h.rerender(h.changes.at(-1));
    assert.equal(h.changes.at(-1).slots[0].startMinute, 120);
    assert.equal(h.changes.at(-1).slots[0].endMinute, 240);
    assert.equal(h.document.body.style.userSelect, 'none', 'updating the schedule must not reset the drag style');
    h.listeners.get('pointerup')(h.pointer(40));
    h.component.settle();
    assert.equal(h.document.body.style.userSelect, 'text', 'release restores the prior style');
  } finally {
    h.component.unmount();
  }
});

test('creating a slot restores selection on unmount during an active drag', () => {
  const h = harness({ ...schedule, slots: [] });
  try {
    h.find(h.component.value, 'data-schedule-day').props.onPointerDown(h.pointer(18));
    assert.equal(h.document.body.style.userSelect, 'text', 'a click alone must not start a drag');
    h.listeners.get('pointermove')(h.pointer(36));
    h.component.settle();
    assert.equal(h.document.body.style.userSelect, 'none');
    assert.equal(h.changes.at(-1).slots[0].startMinute, 60);
    assert.equal(h.changes.at(-1).slots[0].endMinute, 120);
  } finally {
    h.component.unmount();
  }
  assert.equal(h.document.body.style.userSelect, 'text', 'unmount restores the prior style');
  assert.equal(h.listeners.size, 0, 'unmount removes the pointer listeners');
});
