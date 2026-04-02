import React from 'react';
import { Icons } from '../components/Icons';

export type ViewId =
  | 'equipment'
  | 'support'
  | 'castle'
  | 'currency'
  | 'event-modules'
  | 'settings';

export interface NavigationItem {
    id: ViewId;
    label: string;
    icon: React.ReactNode;
    section: 'main' | 'system';
}

export const NAVIGATION_ITEMS: NavigationItem[] = [
    { id: 'castle', label: 'Castle', icon: <Icons.Castle />, section: 'main' },
    { id: 'equipment', label: 'Equipment', icon: <Icons.Shield />, section: 'main' },
    { id: 'event-modules', label: 'Event Modules', icon: <Icons.Trophy />, section: 'main' },
    { id: 'currency', label: 'Currency', icon: <Icons.Grid />, section: 'main' },
    { id: 'settings', label: 'Settings', icon: <Icons.Settings />, section: 'system' },
    { id: 'support', label: 'Support', icon: <Icons.Help />, section: 'system' },
];
