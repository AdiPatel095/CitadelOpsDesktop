// Each baseline entry permits a bounded finding in one owning area and suite.
// Keep raw reports intact; only enforcement consumes the recorded exception.
export function applyGateBaseline(violations, baseline, { area, suite, view, width, locale, theme }) {
  const remaining = [];
  const excluded = [];
  const counts = new Map();
  for (const violation of violations) {
    const index = baseline.findIndex(entry => entry.area === area && entry.suite === suite
      && entry.rule === violation.rule && entry.element === violation.element
      && (!entry.view || entry.view === view) && (!entry.width || entry.width === width)
      && (!entry.locale || entry.locale === locale) && (!entry.theme || entry.theme === theme)
      && (!entry.detail || entry.detail === violation.detail));
    const entry = baseline[index];
    const count = counts.get(index) ?? 0;
    if (entry && Number.isInteger(entry.maximum) && entry.maximum > count && entry.owner && entry.reason) {
      counts.set(index, count + 1);
      excluded.push({ ...violation, owner: entry.owner, reason: entry.reason });
    } else remaining.push(violation);
  }
  return { remaining, excluded };
}
