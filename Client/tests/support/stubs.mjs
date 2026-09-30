// Stand-ins for the parts of the application the draft-recovery hook imports but the hook test does not render.
export const stubState = (globalThis.__stubState ??= { current: null });
// `__stubApi` lets a test supply the API functions the configuration draft session calls.
export const useCitadelAPI = () => ({ state: stubState.current, ...(globalThis.__stubApi ?? {}) });
export const useLocale = () => ({ t: (key) => key });
export class APIError extends Error { constructor(message, code) { super(message); this.code = code; } }
export const Button = () => null;
export const Modal = () => null;
export const LocalizedText = () => null;
export const RotateCcw = () => null;
