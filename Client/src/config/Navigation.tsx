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
    label: string;
    icon: React.ReactNode;
    section: 'main' | 'system';
}

export const NAVIGATION_ITEMS: NavigationItem[] = [
    { id: 'castle', label: 'Castle', icon: <Icons.Castle />, section: 'main' },
    { id: 'automation', label: 'Automation', icon: <Icons.Automation />, section: 'main' },
    { id: 'events', label: 'Feature Stats', icon: <Icons.Trophy />, section: 'main' },
    { id: 'attack-presets', label: 'Attack Presets', icon: <Icons.Crosshair />, section: 'main' },
    { id: 'defense-presets', label: 'Defense Presets', icon: <Icons.Shield />, section: 'main' },
    { id: 'equipment', label: 'Equipment', icon: <Icons.Shield />, section: 'main' },
    { id: 'movement', label: 'Commanders', icon: <Icons.Activity />, section: 'main' },
    { id: 'battle-stats', label: 'Battle Stats', icon: <Icons.Activity />, section: 'main' },
    { id: 'player-tracker', label: 'My Stats', icon: <Icons.Users />, section: 'main' },
    { id: 'alliance-targets', label: 'Alliance Targets', icon: <Icons.Crosshair />, section: 'main' },
	{ id: 'world-intelligence', label: 'World Intel', icon: <Icons.Database />, section: 'main' },
    { id: 'rift', label: 'Rift', icon: <Icons.Rift />, section: 'main' },
    { id: 'settings', label: 'Settings', icon: <Icons.Settings />, section: 'system' },
    { id: 'patch-notes', label: 'Patch Notes', icon: <Icons.PatchNotes />, section: 'system' },
    { id: 'support', label: 'Support', icon: <Icons.Help />, section: 'system' },
];
