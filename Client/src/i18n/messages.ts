import { officialMessageKeys } from './officialKeys';
import { formatMessage, validateMessageCatalog } from './formatMessage';
/** Add explicit descriptors here; never translate user names, IDs, or free-form input. */
export const messages = {
  "support.title": "Support & Community",
  "support.description": "Get help, report issues, or inspect deterministic 2.0 operations.",
  "support.discordTitle": "Join our Discord",
  "support.discordBody": "The best way to get support is to join our Discord server. Our team and community are active and ready to help you with any issues or questions.",
  "support.discordJoin": "Join Discord Server",

  "settings.system": "System Settings",
  "settings.transfer": "Settings Import & Export",
  "settings.export": "Export settings",
  "settings.import": "Import settings",
  "settings.history": "My Stats Storage",
  "settings.username": "Username",
  "settings.password": "Password",
  "settings.server": "Server",
  "settings.browser": "Browser",
  "settings.backgroundLogin": "Save background login",
  "settings.useExecutable": "Use executable",
  "settings.minimumCoins": "Minimum Coins",

  "bot.unlock": "Unlock Bot",
  "bot.lock": "Lock Bot",
  "bot.reconnect": "Reconnect",
  "bot.starting": "Starting\u2026",
  "bot.start": "Start Bot",

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
  "navigation.rift": "Rift Raid",
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
  return validateMessageCatalog(Object.fromEntries(Object.entries(messages).filter(([key]) => !Object.hasOwn(officialMessageKeys,key))), catalog);
}
export function interpolate(template: string, parameters: MessageParameters = {}): string {
  return formatMessage({key: '', fallback: template, params: {...parameters}}, 'en', {}).text;
}
