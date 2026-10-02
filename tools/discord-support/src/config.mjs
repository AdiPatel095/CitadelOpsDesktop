import { homedir } from 'node:os';
import { join, isAbsolute } from 'node:path';
import { privateRead } from './private.mjs';
import { fail } from './errors.mjs';
export const EXPECTED = Object.freeze({
  guildId: '1452045983910461541', botId: '1555353270850027524',
  supportRoleId: '1555357854821523467', appCategoryId: '1555357867819794473',
});
export const basePath = (home = homedir()) => join(home, 'Library/Application Support/CitadelOpsDiscord');
export function validateConfig(raw, home = homedir()) {
  if (!raw || Array.isArray(raw) || typeof raw !== 'object') fail('CONFIG_INVALID');
  const allowed = new Set([...Object.keys(EXPECTED), 'tokenPath']);
  if (Object.keys(raw).some(k => !allowed.has(k))) fail('CONFIG_UNKNOWN_KEY');
  for (const [k, v] of Object.entries(EXPECTED)) if (raw[k] !== v) fail('CONFIG_IDENTITY_MISMATCH');
  const tokenPath = raw.tokenPath ?? join(home, '.config/citadel-ops-discord/bot-token');
  if (typeof tokenPath !== 'string' || !isAbsolute(tokenPath)) fail('CONFIG_TOKEN_PATH_INVALID');
  return { ...EXPECTED, tokenPath };
}
export function readConfig(path, home) {
  let raw;
  try { raw = JSON.parse(privateRead(path)); } catch { fail('CONFIG_UNREADABLE'); }
  return validateConfig(raw, home);
}
// Called only by the service runtime, never by packaging/status/tests against live files.
export function readToken(config) {
  let value;
  try { value = privateRead(config.tokenPath).trim(); } catch { fail('TOKEN_PRIVATE_FILE_REQUIRED'); }
  if (!value || /\s/.test(value)) fail('TOKEN_INVALID');
  return value;
}
