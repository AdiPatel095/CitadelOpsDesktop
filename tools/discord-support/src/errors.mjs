// Never copy exception messages, REST bodies, URLs or customer input into errors.
export class SafeError extends Error {
  constructor(code, fatal = true) { super(code); this.code = code; this.fatal = fatal; }
}
export const fail = (code) => { throw new SafeError(code); };
export function safeCode(error) { return error instanceof SafeError ? error.code : 'OPERATION_FAILED'; }
export function discordError(error) {
  if (error instanceof SafeError) return error;
  if ([10003, 10008, 10004, 10007, 50001, 50013].includes(error?.code)) return new SafeError('DISCORD_RESOURCE_UNAVAILABLE');
  return new SafeError('DISCORD_RETRY_REQUIRED', false);
}
