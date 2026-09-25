import type { MessageKey } from '../i18n/messages';
import React from 'react';
import { Icons } from '../components/Icons';

export type ViewId =
  | 'equipment'
  | 'support'
  | 'castle'
  | 'events'
  | 'attack-presets'
  | 'defense-presets'
  | 'automation'
  | 'movement'
  | 'battle-stats'
  | 'player-tracker'
  | 'alliance-targets'
	| 'world-intelligence'
  | 'rift'
  | 'settings'
  | 'patch-notes';

export interface NavigationItem {
    id: ViewId;
    labelKey: MessageKey;
    icon: React.ReactNode;
    section: 'main' | 'system';
}

export const NAVIGATION_ITEMS: NavigationItem[] = [
    { id: 'castle', labelKey: 'navigation.castle', icon: <Icons.Castle />, section: 'main' },
    { id: 'automation', labelKey: 'navigation.automation', icon: <Icons.Automation />, section: 'main' },
    { id: 'events', labelKey: 'navigation.events', icon: <Icons.Trophy />, section: 'main' },
    { id: 'attack-presets', labelKey: 'navigation.attack-presets', icon: <Icons.Crosshair />, section: 'main' },
    { id: 'defense-presets', labelKey: 'navigation.defense-presets', icon: <Icons.Shield />, section: 'main' },
    { id: 'equipment', labelKey: 'navigation.equipment', icon: <Icons.Shield />, section: 'main' },
    { id: 'movement', labelKey: 'navigation.movement', icon: <Icons.Activity />, section: 'main' },
    { id: 'battle-stats', labelKey: 'navigation.battle-stats', icon: <Icons.Activity />, section: 'main' },
    { id: 'player-tracker', labelKey: 'navigation.player-tracker', icon: <Icons.Users />, section: 'main' },
    { id: 'alliance-targets', labelKey: 'navigation.alliance-targets', icon: <Icons.Crosshair />, section: 'main' },
	{ id: 'world-intelligence', labelKey: 'navigation.world-intelligence', icon: <Icons.Database />, section: 'main' },
    { id: 'rift', labelKey: 'navigation.rift', icon: <Icons.Rift />, section: 'main' },
    { id: 'settings', labelKey: 'navigation.settings', icon: <Icons.Settings />, section: 'system' },
    { id: 'patch-notes', labelKey: 'navigation.patch-notes', icon: <Icons.PatchNotes />, section: 'system' },
    { id: 'support', labelKey: 'navigation.support', icon: <Icons.Help />, section: 'system' },
];
