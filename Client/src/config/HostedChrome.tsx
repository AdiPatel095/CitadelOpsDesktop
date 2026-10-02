/* eslint react-refresh/only-export-components: ["error", { "allowExportNames": ["setHostedHeaderChrome", "setHostedConnectionActions"] }] */
/**
 * Host-injected header chrome.
 *
 * When the command center runs inside the customer portal it is the only
 * header on screen, so the portal needs to place account-level controls
 * (Account Center, active account switcher) into it. Rather than teaching the
 * desktop header about portal routing and contexts, the portal registers a
 * component here and the header renders it through `HostedHeaderChromeSlot`.
 *
 * On the desktop nothing is registered and the slot renders nothing, which
 * keeps this file safe to sync back into the desktop repository.
 *
 * Registration happens at module scope in the portal host, before the command
 * center is imported, so the value is stable by first render.
 */
import type { ComponentType } from 'react';

let hostedHeaderChrome: ComponentType | null = null;

export const setHostedHeaderChrome = (component: ComponentType | null) => {
    hostedHeaderChrome = component;
};

export const HostedHeaderChromeSlot = () => {
    const Chrome = hostedHeaderChrome;
    return Chrome ? <Chrome /> : null;
};

let hostedConnectionActions: ComponentType | null = null;
export const setHostedConnectionActions = (component: ComponentType | null) => { hostedConnectionActions = component; };
export const HostedConnectionActionsSlot = () => { const Actions = hostedConnectionActions; return Actions ? <Actions /> : null; };
