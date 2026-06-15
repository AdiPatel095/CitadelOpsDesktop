import React from 'react';
import { Icons } from '../components/Icons';
import { Badge, Card, CardContent, CardHeader, CardTitle } from '../components/ui';
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
    <Card
      className={
        isLatest
          ? 'border-primary/25 shadow-[0_0_24px_-8px_rgba(52,211,153,0.35)]'
          : 'border-border-base opacity-95'
      }
    >
      <CardHeader className="pb-2 border-b border-border-base/80">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <CardTitle className="text-xl flex items-center gap-2">
            <span className="font-mono text-primary">v{release.version}</span>
            {isLatest && (
              <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-full bg-primary/15 text-primary border border-primary/30">
                Current
              </span>
            )}
          </CardTitle>
          {release.date && (
            <span className="text-xs text-text-muted font-mono">{release.date}</span>
          )}
        </div>
        {release.subtitle && (
          <p className="text-sm text-text-muted mt-1 leading-relaxed">{release.subtitle}</p>
        )}
      </CardHeader>
      <CardContent className="pt-5">
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
      </CardContent>
    </Card>
  );
}

const PatchNotesView: React.FC = () => {
  return (
    <div className="max-w-3xl mx-auto py-6 pb-16">
      <div className="flex items-start gap-4 mb-8">
        <div className="w-14 h-14 rounded-2xl bg-primary/10 border border-primary/20 flex items-center justify-center shrink-0">
          <Icons.PatchNotes className="w-7 h-7 text-primary" />
        </div>
        <div>
          <h1 className="text-3xl font-bold tracking-tight text-text-main">Patch Notes</h1>
          <p className="text-text-muted mt-1 text-base leading-relaxed">
            Summaries of recent updates. You’re on version{' '}
            <span className="font-mono text-text-main">v{APP_VERSION_CURRENT}</span>.
          </p>
        </div>
      </div>

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
