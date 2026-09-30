// Stand-ins for the parts of the application the draft-recovery hook imports but the hook test does not render.
export const stubState = (globalThis.__stubState ??= { current: null });
export const useCitadelAPI = () => ({ state: stubState.current });
export const Button = () => null;
export const Modal = () => null;
export const LocalizedText = () => null;
export const RotateCcw = () => null;
