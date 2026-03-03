import React from 'react';
import { Icons } from '../components/Icons';

export type ViewId = 'equipment' | 'support' | 'dashboard' | 'currency' | 'units' | 'event-modules';

export interface NavigationItem {
    id: ViewId;
    label: string;
    icon: React.ReactNode;
    section: 'main' | 'system';
}

export const NAVIGATION_ITEMS: NavigationItem[] = [
    { id: 'dashboard', label: 'Dashboard', icon: <Icons.Database />, section: 'main' },
    { id: 'units', label: 'Units', icon: <Icons.Users />, section: 'main' },
    { id: 'equipment', label: 'Equipment', icon: <Icons.Shield />, section: 'main' },
    { id: 'event-modules', label: 'Event Modules', icon: <Icons.Trophy />, section: 'main' },
    { id: 'currency', label: 'Currency', icon: <Icons.Grid />, section: 'main' },
    { id: 'support', label: 'Support', icon: <Icons.Help />, section: 'system' },
];
