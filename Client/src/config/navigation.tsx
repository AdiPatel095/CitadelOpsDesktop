import React from 'react';
import { Icons } from '../components/Icons';

export type ViewId = 'equipment' | 'settings' | 'support';

export interface NavigationItem {
    id: ViewId;
    label: string;
    icon: React.ReactNode;
    section: 'main' | 'system';
}

export const NAVIGATION_ITEMS: NavigationItem[] = [
    { id: 'equipment', label: 'Equipment', icon: <Icons.Shield />, section: 'main' },
    { id: 'settings', label: 'Settings', icon: <Icons.Settings />, section: 'system' },
    { id: 'support', label: 'Support', icon: <Icons.Help />, section: 'system' },
];
