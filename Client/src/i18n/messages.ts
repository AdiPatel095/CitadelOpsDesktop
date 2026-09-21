import { formatMessage, validateMessageCatalog } from './formatMessage';
/** Add explicit descriptors here; never translate user names, IDs, or free-form input. */
export const messages = {
  "navigation.castle": "Castle",
  "navigation.automation": "Automation",
  "navigation.events": "Feature Stats",
  "navigation.attack-presets": "Attack Presets",
  "navigation.defense-presets": "Defense Presets",
  "navigation.equipment": "Equipment",
  "navigation.movement": "Commanders",
  "navigation.battle-stats": "Battle Stats",
  "navigation.player-tracker": "My Stats",
  "navigation.alliance-targets": "Alliance Targets",
  "navigation.world-intelligence": "World Intel",
  "navigation.rift": "Rift",
  "navigation.settings": "Settings",
  "navigation.patch-notes": "Patch Notes",
  "navigation.support": "Support",
  "navigation.close": "Close workspace navigation",
  "navigation.application": "Application navigation",
  "navigation.primary": "Primary",
  "navigation.system": "System",
  "navigation.commandCenter": "Command center",
  "navigation.workspace": "Workspace",
  "navigation.operations": "Operations",
  "theme.toggle": "Toggle Theme",
  "theme.switchDark": "Switch to Dark Mode",
  "theme.switchLight": "Switch to Light Mode",
  "settings.cancel": "Cancel",
  "settings.save": "Save settings",
  "settings.toggle": "Toggle setting",
  "workspace.loading": "Loading workspace…",

  'locale.select': 'Display language',
  'locale.coverage': 'Some interface text is still English.',
} as const;
export type MessageKey = keyof typeof messages;
export type MessageParameters = Readonly<Record<string, string | number>>;
export type MessageDescriptor = { key: MessageKey; parameters?: MessageParameters };
export type MessageCatalog = Record<MessageKey, string>;
export function validateCatalog(catalog: Record<string, string>): string[] {
  return validateMessageCatalog(messages, catalog);
}
export function interpolate(template: string, parameters: MessageParameters = {}): string {
  return formatMessage({key: '', fallback: template, params: {...parameters}}, 'en', {}).text;
}
