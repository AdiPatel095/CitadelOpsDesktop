import type { ReactNode } from 'react';
import type { ReadinessCheck } from '../readiness/Readiness';
import type { SettingsDisclosure } from '../disclosure/useSettingsDisclosure';
import { CollapsedSettingNote } from './SettingsSection';

/**
 * Readiness note for a blocked or waiting check whose control is inside a
 * collapsed Advanced section: the control's current value and where it lives.
 * `values` maps control ids to their current display value.
 */
export function collapsedSettingNote(
  disclosure: SettingsDisclosure,
  values: Readonly<Record<string, ReactNode>> = {},
): (check: ReadinessCheck) => ReactNode {
  return (check) => {
    const collapsed = disclosure.collapsedTarget(check);
    if (!collapsed) return null;
    return <CollapsedSettingNote collapsed={collapsed} value={values[collapsed.target.control]} />;
  };
}
