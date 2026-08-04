import React from 'react';
import { Icons } from '../components/Icons';
import { Badge, PageHeader, SectionCard } from '../components/ui';
import {
  APP_VERSION_CURRENT,
  PATCH_NOTE_KIND_LABEL,
  PATCH_NOTES_RELEASES,
  type PatchNoteKind,
  type PatchNotesRelease,
} from '../config/PatchNotes';
import type { BadgeProps } from '../components/ui/Badge';

const PATCH_NOTE_BADGE_VARIANT: Record<PatchNoteKind, NonNullable<BadgeProps['variant']>> = {
  added: 'primary',
  fixed: 'success',
  removed: 'danger',
  changed: 'warning',
  security: 'outline',
  deprecated: 'secondary',
};

function ReleaseCard({ release, isLatest }: { release: PatchNotesRelease; isLatest: boolean }) {
  return (
    <SectionCard
      variant="solid"
      title={(
        <>
          <span className="font-mono text-primary">v{release.version}</span>
          {isLatest && <Badge variant="primary">Current</Badge>}
        </>
      )}
      description={release.subtitle}
      actions={release.date ? <span className="font-mono text-xs text-text-muted">{release.date}</span> : undefined}
      titleClassName="text-xl"
      className={
        isLatest
          ? 'liquid-prominent-header-card border-primary/25 shadow-[0_0_24px_-8px_var(--primary-glow)]'
          : 'liquid-prominent-header-card border-border-base opacity-95'
      }
    >
        {release.items.length > 0 && (
          <ul className="space-y-3 text-sm text-text-main leading-relaxed">
            {release.items.map((item, idx) => (
              <li key={`${release.version}-${idx}`} className="flex gap-3 items-start">
                <Badge variant={PATCH_NOTE_BADGE_VARIANT[item.kind]} className="shrink-0 mt-0.5">
                  {PATCH_NOTE_KIND_LABEL[item.kind]}
                </Badge>
                <span className="min-w-0 pt-0.5">{item.text}</span>
              </li>
            ))}
          </ul>
        )}
    </SectionCard>
  );
}

const PatchNotesView: React.FC = () => {
  return (
    <div className="max-w-3xl mx-auto py-6 pb-16">
      <PageHeader
        className="mb-8"
        title="Patch Notes"
        icon={<Icons.PatchNotes className="h-7 w-7" />}
        description={<>Summaries of recent updates. You’re on version <span className="font-mono text-text-main">v{APP_VERSION_CURRENT}</span>.</>}
      />

      <div className="space-y-8">
        {PATCH_NOTES_RELEASES.map((release, i) => (
          <ReleaseCard key={release.version} release={release} isLatest={i === 0} />
        ))}
      </div>

      <p className="mt-10 text-xs text-text-muted text-center">
        Earlier versions will appear here as they’re released.
      </p>
    </div>
  );
};

export default PatchNotesView;
