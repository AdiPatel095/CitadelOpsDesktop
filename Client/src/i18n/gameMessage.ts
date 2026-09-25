/** Official templates are not ICU. Substitute once; inserted user values are never parsed. */
export function formatGameMessage(template: string, parameters: readonly (string | number | undefined)[] = [], locale = 'en'): string {
  return template.replace(/\{(\d+)\}/g, (token, index: string) => {
    const value = parameters[Number(index)];
    if (value === undefined) return token;
    const text = String(value);
    return locale === 'ar' && text ? `\u2068${text}\u2069` : text;
  });
}
