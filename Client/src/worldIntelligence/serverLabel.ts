export type ServerLabelEntry = { code: string; label?: string; host?: string; url?: string };

const codeOnly = (value: string) => /^[a-z][a-z0-9_-]*\d$/i.test(value.trim()) ? value.trim().toUpperCase() : '';
const hostname = (value: string) => {
  try { return new URL(value.includes('://') ? value : `https://${value}`).hostname.toLowerCase(); }
  catch { return ''; }
};

/** Display only: never use this label as a world ID or a request parameter. */
export function serverLabel(value: string, servers: readonly ServerLabelEntry[], accountCode = ''): string {
  const code = codeOnly(value);
  const ownCode = codeOnly(accountCode);
  const host = hostname(value);
  const matches = servers.filter(server => hostname(server.host || server.url || '') === host);
  const server = code
    ? servers.find(server => server.code.toUpperCase() === code)
    : matches.find(server => server.code.toUpperCase() === ownCode) ?? (matches.length === 1 ? matches[0] : matches.length === 0 ? servers.find(server => server.code.toUpperCase() === ownCode) : undefined);
  const resolvedCode = server?.code.toUpperCase() || code || ownCode;
  const name = server?.label?.trim();
  return name && name.toUpperCase() !== resolvedCode && !/[/:]|\.[a-z]{2,}/i.test(name)
    ? `${name} (${resolvedCode})`
    : resolvedCode;
}
