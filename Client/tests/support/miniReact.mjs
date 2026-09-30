/**
 * A very small stand-in for React, used only by the draft-recovery hook test (CIT-19). It runs the real hook code with the
 * parts of React the hook uses (state, refs, memo, callbacks, passive effects with dependency arrays and cleanups) and
 * lets the test decide WHEN a state update made inside an effect is rendered, which is exactly the ordering that broke the
 * old timer-based baseline: in a browser the update from an editor's load effect renders in a later task, after a
 * zero-delay timer registered in the same effects flush.
 *
 * It is not React: no scheduler, no context, no concurrent rendering. Elements are plain objects and never rendered.
 */
// One shared cell: this file is loaded twice (natively by the test, and through Vite for the hook), and both must agree.
const cell = (globalThis.__miniReact ??= { active: null });

const same = (left, right) => left.length === right.length && left.every((value, index) => Object.is(value, right[index]));

export function mount(component, { strict = false } = {}) {
  const instance = { hooks: [], cursor: 0, pending: [], dirty: false, props: undefined, value: undefined, unmounted: false };
  const render = () => {
    cell.active = instance;
    instance.cursor = 0;
    instance.pending = [];
    instance.dirty = false;
    try { instance.value = component(instance.props); } finally { cell.active = null; }
  };
  const commit = () => {
    const runs = instance.pending.filter((slot) => slot.run);
    for (const slot of runs) { if (typeof slot.cleanup === 'function') { slot.cleanup(); } slot.cleanup = undefined; }
    for (const slot of runs) { slot.cleanup = slot.effect(); slot.run = false; }
  };
  const handle = {
    /** Render with new props and run the passive effects, without rendering again the updates the effects made. */
    render(props) {
      instance.props = props;
      render();
      commit();
      if (strict && !handle.strictDone) {
        handle.strictDone = true;
        for (const slot of instance.hooks) if (slot && slot.effect) { if (typeof slot.cleanup === 'function') slot.cleanup(); slot.cleanup = slot.effect(); }
      }
      return instance.value;
    },
    /** Render until no effect leaves an update behind. */
    settle(limit = 20) {
      let count = 0;
      while (instance.dirty && count < limit) { count += 1; render(); commit(); }
      if (instance.dirty) throw new Error('did not settle');
      return instance.value;
    },
    get value() { return instance.value; },
    get pendingUpdate() { return instance.dirty; },
    unmount() {
      instance.unmounted = true;
      for (const slot of instance.hooks) if (slot && typeof slot.cleanup === 'function') { slot.cleanup(); slot.cleanup = undefined; }
    },
  };
  return handle;
}

const slot = () => {
  const instance = cell.active;
  if (!instance) throw new Error('hook called outside a render');
  const index = instance.cursor;
  instance.cursor += 1;
  return { instance, index, existing: instance.hooks[index] };
};

export function useState(initial) {
  const { instance, index, existing } = slot();
  if (existing) return [existing.value, existing.set];
  const entry = { value: typeof initial === 'function' ? initial() : initial };
  entry.set = (next) => {
    const value = typeof next === 'function' ? next(entry.value) : next;
    if (Object.is(value, entry.value)) return;
    entry.value = value;
    instance.dirty = true;
  };
  instance.hooks[index] = entry;
  return [entry.value, entry.set];
}

export function useRef(initial) {
  const { instance, index, existing } = slot();
  if (existing) return existing.ref;
  const entry = { ref: { current: initial } };
  instance.hooks[index] = entry;
  return entry.ref;
}

export function useMemo(factory, deps) {
  const { instance, index, existing } = slot();
  if (existing && deps && existing.deps && same(existing.deps, deps)) return existing.value;
  const entry = { value: factory(), deps };
  instance.hooks[index] = entry;
  return entry.value;
}

export const useCallback = (callback, deps) => useMemo(() => callback, deps);

export function useEffect(effect, deps) {
  const { instance, index, existing } = slot();
  const entry = existing ?? { cleanup: undefined, deps: undefined };
  const changed = !existing || !deps || !entry.deps || !same(entry.deps, deps);
  entry.effect = effect;
  entry.deps = deps;
  entry.run = changed;
  instance.hooks[index] = entry;
  instance.pending.push(entry);
}
export const useLayoutEffect = useEffect;
export const useSyncExternalStore = (_subscribe, getSnapshot) => getSnapshot();
export const useId = () => 'id';
export const createElement = (type, props, ...children) => ({ type, props: { ...props, children } });
export const Fragment = Symbol('Fragment');
export const jsx = (type, props) => ({ type, props });
export const jsxs = jsx;
export const jsxDEV = jsx;
export default { useState, useRef, useMemo, useCallback, useEffect, useLayoutEffect, createElement, Fragment };
