import React from 'react';
import { Icons } from '../components/Icons';
import { Badge, PageHeader, SectionCard } from '../components/ui';
import {
  APP_VERSION_CURRENT,
  PATCH_NOTE_KIND_LABEL,
  PATCH_NOTE_KIND_ORDER,
  PATCH_NOTES_RELEASES,
  type PatchNoteKind,
  type PatchNotesRelease,
} from '../config/PatchNotes';
import type { BadgeProps } from '../components/ui/Badge';

const PATCH_NOTE_BADGE_VARIANT: Record<PatchNoteKind, NonNullable<BadgeProps['variant']>> = {
  added: 'primary',
  fixed: 'success',
  security: 'outline',
  changed: 'warning',
  removed: 'danger',
  deprecated: 'secondary',
};

function ReleaseCard({ release, isLatest }: { release: PatchNotesRelease; isLatest: boolean }) {
  const groups = PATCH_NOTE_KIND_ORDER
    .map((kind) => ({
      kind,
      items: release.items.filter((item) => item.kind === kind),
    }))
    .filter((group) => group.items.length > 0);

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
      {groups.length > 0 && (
        <div className="space-y-6">
          {groups.map((group) => (
            <section key={`${release.version}-${group.kind}`} aria-labelledby={`${release.version}-${group.kind}-heading`}>
              <div className="mb-3 flex items-center gap-2">
                <Badge
                  id={`${release.version}-${group.kind}-heading`}
                  variant={PATCH_NOTE_BADGE_VARIANT[group.kind]}
                  className="shrink-0"
                >
                  {PATCH_NOTE_KIND_LABEL[group.kind]}
                </Badge>
                <span className="text-xs tabular-nums text-text-muted">
                  {group.items.length} {group.items.length === 1 ? 'change' : 'changes'}
                </span>
              </div>
              <ul className="space-y-3 text-sm leading-relaxed text-text-main">
                {group.items.map((item, index) => (
                  <li key={`${release.version}-${group.kind}-${index}`} className="flex items-start gap-3">
                    <span aria-hidden="true" className="mt-[0.45rem] h-1.5 w-1.5 shrink-0 rounded-full bg-current opacity-55" />
                    <span className="min-w-0">{item.text}</span>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
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
