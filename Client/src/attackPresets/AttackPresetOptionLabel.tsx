import React from 'react';
import { Badge } from '../components/ui/Badge';
import type { SelectOption } from '../components/ui/Select';
import { LocalizedText } from '../i18n/LocalizedText';

/** Any preset record listed in a picker (attack or defense). */
interface ListedPreset {
  id: string;
  name: string;
  app?: unknown;
}

/** "Created by app" marker shown wherever an app-created preset is listed. */
export function appCreatedPresetBadge(className = ''): React.ReactElement {
  return (
    <Badge variant="secondary" className={`shrink-0 normal-case tracking-normal ${className}`}>
      <LocalizedText messageKey="attackPresets.createdByApp" />
    </Badge>
  );
}

/** Select option label: the preset name, plus the badge for app-created presets. */
export function presetOptionLabel(preset: ListedPreset, detail?: string): React.ReactNode {
  const text = detail ? `${preset.name} · ${detail}` : preset.name;
  if (!preset.app) return text;
  return (
    <span className="flex min-w-0 items-center gap-2">
      <span className="truncate">{text}</span>
      {appCreatedPresetBadge()}
    </span>
  );
}

/** Options for a preset picker. Search keeps matching the plain preset name. */
export function attackPresetSelectOptions<T extends ListedPreset>(
  presets: readonly T[],
  describe?: (preset: T) => string,
): SelectOption[] {
  return presets.map((preset) => {
    const detail = describe?.(preset);
    return {
      value: preset.id,
      label: presetOptionLabel(preset, detail),
      searchText: detail ? `${preset.name} · ${detail}` : preset.name,
    };
  });
}
