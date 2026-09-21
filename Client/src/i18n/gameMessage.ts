/** Official templates are not ICU. Substitute once; inserted user values are never parsed. */
export function formatGameMessage(template: string, parameters: readonly (string | number)[] = []): string {
  return template.replace(/\{(\d+)\}/g, (token, index: string) => {
    const value = parameters[Number(index)];
    return value === undefined ? token : String(value);
  });
}
