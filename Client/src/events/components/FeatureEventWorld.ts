/** Match the cloud API's canonical world identity without merging distinct ports. */
export function canonicalEventWorldID(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return '';
  const hasScheme = trimmed.includes('://');
  try {
    const url = new URL(hasScheme ? trimmed : `https://${trimmed}`);
    if (!['http:', 'https:', 'ws:', 'wss:'].includes(url.protocol) || url.username || url.password) return '';
    const port = !hasScheme && ['80', '443'].includes(url.port) ? '' : url.port;
    return url.hostname.toLowerCase() + (port ? `:${port}` : '');
  } catch {
    return '';
  }
}

export function featureHistoryMatchesScope(history: unknown, worldId: string, playerId: number): boolean {
  const world = canonicalEventWorldID(worldId);
  if (!world || !Number.isSafeInteger(playerId) || playerId <= 0 || !history || typeof history !== 'object') return false;
  const value = history as { worldId?: unknown; playerId?: unknown; history?: unknown };
  return value.playerId === playerId && typeof value.worldId === 'string'
    && canonicalEventWorldID(value.worldId) === world && Array.isArray(value.history);
}
