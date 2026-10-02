import React, { type ReactNode } from 'react';
import { ChevronDown, ChevronRight, SlidersHorizontal } from 'lucide-react';
import { Badge } from '../../components/ui/Badge';
import { useLocale } from '../../i18n/LocaleContext';
import { LocalizedText } from '../../i18n/LocalizedText';
import type { ReadinessCheck } from '../readiness/Readiness';
import { sectionPlacement, settingsSectionElementId } from '../disclosure/placement';
import type { SettingsSummaryLine } from '../disclosure/summaries';
import type { CollapsedFixTarget, SettingsDisclosure } from '../disclosure/useSettingsDisclosure';

export interface SettingsSectionProps {
  disclosure: SettingsDisclosure;
  /** Section id from `SETTINGS_PLACEMENT`; the tier and title come from there. */
  section: string;
  /** Always visible while an Advanced section is collapsed. */
  summary?: readonly SettingsSummaryLine[];
  /** Number of values that differ from the feature defaults. */
  customCount?: number;
  className?: string;
  children: ReactNode;
}

/**
 * One Essentials or Advanced group of a settings modal (CIT-17). Essentials
 * render as-is. Advanced groups get a disclosure header and a summary line;
 * collapsing only hides the body (it stays mounted), so nothing is reset,
 * saved, started or stopped by expanding or collapsing.
 */
export const SettingsSection: React.FC<SettingsSectionProps> = ({ disclosure, section, summary, customCount = 0, className = '', children }) => {
  const placement = sectionPlacement(disclosure.featureId, section);
  const elementId = settingsSectionElementId(disclosure.featureId, section);
  if (placement.tier === 'essentials') {
    return (
      <div id={elementId} data-settings-section={section} data-settings-tier="essentials" className={className}>
        {children}
      </div>
    );
  }
  const expanded = disclosure.isExpanded(section);
  const bodyId = `${elementId}-body`;
  const headingId = `${elementId}-heading`;
  return (
    <section
      id={elementId}
      data-settings-section={section}
      data-settings-tier="advanced"
      aria-labelledby={headingId}
      className={`rounded-global border border-dashed border-border-base bg-bg-card/30 ${className}`}
    >
      <button data-button-pattern="disclosure"
        type="button"
        className="flex w-full items-start justify-between gap-3 px-4 py-3 text-left"
        aria-expanded={expanded}
        aria-controls={bodyId}
        onClick={() => disclosure.toggle(section)}
      >
        <span className="min-w-0">
          <span className="flex flex-wrap items-center gap-2">
            <SlidersHorizontal className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
            <span id={headingId} className="text-body font-semibold text-text-main"><LocalizedText messageKey={placement.titleKey} /></span>
            <Badge variant="outline" className="normal-case"><LocalizedText messageKey="ui.settings.components.settingsSection.advanced.9f088dbe" /></Badge>
            {customCount > 0 ? (
              <Badge variant="warning" className="normal-case"><LocalizedText messageKey="settingsSummary.customValues" params={{ count: customCount }} /></Badge>
            ) : null}
          </span>
          {!expanded && summary && summary.length > 0 ? (
            <span className="mt-1 block text-caption text-text-muted">
              {summary.map((line, index) => (
                <React.Fragment key={`${line.messageKey}:${index}`}>
                  {index > 0 ? ' · ' : null}
                  <LocalizedText messageKey={line.messageKey} params={line.params} />
                </React.Fragment>
              ))}
            </span>
          ) : null}
        </span>
        <span className="flex shrink-0 items-center gap-1 text-caption font-semibold text-text-muted">
          {expanded
            ? <LocalizedText messageKey="ui.settings.components.settingsSection.hide.ac20a57b" />
            : <LocalizedText messageKey="ui.settings.components.settingsSection.show.and.edit.f60aed35" />}
          {expanded ? <ChevronDown className="h-4 w-4" aria-hidden="true" /> : <ChevronRight className="h-4 w-4 rtl:-scale-x-100" aria-hidden="true" />}
        </span>
      </button>
      <div id={bodyId} hidden={!expanded} className="border-t border-border-base px-4 py-4">
        {children}
      </div>
    </section>
  );
};

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

const CollapsedSettingNote: React.FC<{ collapsed: CollapsedFixTarget; value: ReactNode }> = ({ collapsed, value }) => {
  const { t } = useLocale();
  return (
    <span className="block text-caption text-text-muted">
      {collapsed.target.labelKey && value != null ? (
        <span className="font-semibold text-text-main"><LocalizedText messageKey={collapsed.target.labelKey} />{': '}{value}{' · '}</span>
      ) : null}
      <LocalizedText messageKey="settingsSummary.collapsedIn" params={{ section: t(collapsed.placement.titleKey) }} />
    </span>
  );
};
