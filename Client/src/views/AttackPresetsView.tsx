import React, { Suspense, useMemo, useState } from 'react';
import { Copy, Edit3, Library, Plus, Shield, Swords, Trash2 } from 'lucide-react';
import { useCitadelAPI } from '../api/ApiContext';
import type { AttackSetupDraft } from '../components/AttackSetupModal';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CollectionToolbar,
  EmptyState,
  MetricTile,
  PageHeader,
} from '../components/ui';
import { Notifications } from '../components/Notifications';
import ToolImage from '../components/ToolImage';
import UnitImage from '../components/UnitImage';
import {
  ATTACK_PRESETS_SECTION,
  type AppAttackPreset,
  parseAttackPresetDocument,
  summarizeAttackPreset,
} from '../attackPresets/AttackPresetTypes';

const AttackSetupModal = React.lazy(() => import('../components/AttackSetupModal'));

interface EditorState {
  presetID: string | null;
  draft?: AttackSetupDraft;
}

const AttackPresetsView: React.FC = () => {
  const { configuration, updateConfiguration } = useCitadelAPI();
  const [query, setQuery] = useState('');
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [saving, setSaving] = useState(false);
  const [pendingID, setPendingID] = useState<string | null>(null);

  const document = useMemo(
    () => parseAttackPresetDocument(configuration?.sections[ATTACK_PRESETS_SECTION]),
    [configuration?.sections],
  );
  const filteredPresets = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    if (!normalized) return document.presets;
    return document.presets.filter((preset) => preset.name.toLowerCase().includes(normalized));
  }, [document.presets, query]);

  const saveDocument = async (presets: AppAttackPreset[], successMessage: string) => {
    await updateConfiguration(ATTACK_PRESETS_SECTION, { version: 1, presets });
    Notifications.success(successMessage);
  };

  const handleSave = async (draft: AttackSetupDraft) => {
    if (saving) return;
    setSaving(true);
    const now = new Date().toISOString();
    const existing = editor?.presetID
      ? document.presets.find((preset) => preset.id === editor.presetID)
      : undefined;
    const preset: AppAttackPreset = {
      id: existing?.id ?? createID(),
      name: draft.name.trim(),
      waves: draft.waves,
      createdAt: existing?.createdAt ?? now,
      updatedAt: now,
    };
    const presets = existing
      ? document.presets.map((candidate) => candidate.id === existing.id ? preset : candidate)
      : [...document.presets, preset];
    try {
      await saveDocument(presets, existing ? 'Attack preset updated.' : 'Attack preset created.');
      setEditor(null);
    } catch (error) {
      Notifications.error(errorMessage(error, 'Could not save attack preset.'));
    } finally {
      setSaving(false);
    }
  };

  const handleDuplicate = async (preset: AppAttackPreset) => {
    if (pendingID) return;
    setPendingID(preset.id);
    const now = new Date().toISOString();
    const duplicate: AppAttackPreset = {
      ...preset,
      id: createID(),
      name: uniqueCopyName(preset.name, document.presets),
      waves: cloneWaves(preset.waves),
      createdAt: now,
      updatedAt: now,
    };
    try {
      await saveDocument([...document.presets, duplicate], 'Attack preset duplicated.');
    } catch (error) {
      Notifications.error(errorMessage(error, 'Could not duplicate attack preset.'));
    } finally {
      setPendingID(null);
    }
  };

  const handleDelete = async (preset: AppAttackPreset) => {
    if (pendingID || !window.confirm(`Delete “${preset.name}”? This cannot be undone.`)) return;
    setPendingID(preset.id);
    try {
      await saveDocument(
        document.presets.filter((candidate) => candidate.id !== preset.id),
        'Attack preset deleted.',
      );
    } catch (error) {
      Notifications.error(errorMessage(error, 'Could not delete attack preset.'));
    } finally {
      setPendingID(null);
    }
  };

  return (
    <div className="mx-auto flex w-full max-w-[1800px] flex-col gap-5 pb-10">
      <PageHeader
        title="Attack Presets"
        description="Build reusable multi-wave formations for manual attacks and automation."
        icon={<Library className="h-5 w-5" />}
        actions={(
          <Button leftIcon={<Plus className="h-4 w-4" />} onClick={() => setEditor({ presetID: null })}>
            New preset
          </Button>
        )}
      />

      <CollectionToolbar
        summary={(
          <>
          <Badge variant={document.presets.length > 0 ? 'primary' : 'secondary'}>
            {document.presets.length} preset{document.presets.length === 1 ? '' : 's'}
          </Badge>
          <Badge variant="outline" className="normal-case tracking-normal">Stored by CitadelOps</Badge>
          </>
        )}
        searchValue={query}
        onSearchChange={setQuery}
        searchPlaceholder="Search presets"
      />

      {filteredPresets.length > 0 ? (
        <div className="grid gap-4 xl:grid-cols-2">
          {filteredPresets.map((preset) => (
            <PresetCard
              key={preset.id}
              preset={preset}
              busy={pendingID === preset.id}
              onEdit={() => setEditor({ presetID: preset.id, draft: preset })}
              onDuplicate={() => void handleDuplicate(preset)}
              onDelete={() => void handleDelete(preset)}
            />
          ))}
        </div>
      ) : (
        <EmptyState
          size="lg"
          icon={<Swords className="h-6 w-6" />}
          title={query.trim() ? 'No matching presets' : 'Create your first attack preset'}
          description={query.trim()
            ? 'Try a different preset name.'
            : 'Presets are independent from the game’s saved slots and can contain up to 30 complete attack waves.'}
          action={!query.trim() ? (
            <Button leftIcon={<Plus className="h-4 w-4" />} onClick={() => setEditor({ presetID: null })}>
              Create preset
            </Button>
          ) : undefined}
        />
      )}

      {editor ? (
        <Suspense fallback={null}>
          <AttackSetupModal
            isOpen
            initialDraft={editor.draft}
            inventoryPolicy="advisory"
            onClose={() => { if (!saving) setEditor(null); }}
            onSave={(draft) => void handleSave(draft)}
          />
        </Suspense>
      ) : null}
    </div>
  );
};

const PresetCard: React.FC<{
  preset: AppAttackPreset;
  busy: boolean;
  onEdit: () => void;
  onDuplicate: () => void;
  onDelete: () => void;
}> = ({ preset, busy, onEdit, onDuplicate, onDelete }) => {
  const summary = summarizeAttackPreset(preset);
  return (
    <Card variant="solid" className="liquid-prominent-header-card overflow-hidden">
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border-base bg-bg-card/45 px-5 py-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <Swords className="h-4 w-4 shrink-0 text-primary" />
            <h2 className="truncate text-base font-black text-text-main">{preset.name}</h2>
          </div>
          <p className="mt-1 text-xs text-text-muted">Updated {formatUpdatedAt(preset.updatedAt)}</p>
        </div>
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="icon" disabled={busy} onClick={onEdit} title="Edit preset"><Edit3 className="h-4 w-4" /></Button>
          <Button variant="ghost" size="icon" disabled={busy} onClick={onDuplicate} title="Duplicate preset"><Copy className="h-4 w-4" /></Button>
          <Button variant="ghost" size="icon" disabled={busy} onClick={onDelete} title="Delete preset" className="hover:!text-error"><Trash2 className="h-4 w-4" /></Button>
        </div>
      </div>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-3 gap-2">
          <MetricTile label="Waves" value={summary.waves.toLocaleString()} />
          <MetricTile label="Troops" value={summary.troops.toLocaleString()} />
          <MetricTile label="Tools" value={summary.tools.toLocaleString()} />
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <FormationPreview label="Unit types" ids={summary.troopTypes} emptyIcon={<Shield className="h-4 w-4" />} render={(id) => <UnitImage unitId={id} size={34} />} />
          <FormationPreview label="Tool types" ids={summary.toolTypes} emptyIcon={<Shield className="h-4 w-4" />} render={(id) => <ToolImage toolId={id} size={34} showLevel={false} />} />
        </div>
        <Button variant="secondary" className="w-full" onClick={onEdit} disabled={busy} leftIcon={<Edit3 className="h-4 w-4" />}>
          Edit formation
        </Button>
      </CardContent>
    </Card>
  );
};

const FormationPreview: React.FC<{
  label: string;
  ids: number[];
  emptyIcon: React.ReactNode;
  render: (id: number) => React.ReactNode;
}> = ({ label, ids, emptyIcon, render }) => (
  <div className="min-w-0 rounded-global border border-border-base bg-bg-app/35 p-3">
    <div className="mb-2 text-[9px] font-black uppercase tracking-wider text-text-muted">{label}</div>
    <div className="flex min-h-[2.125rem] items-center gap-1.5 overflow-hidden">
      {ids.length > 0 ? ids.slice(0, 7).map((id) => <React.Fragment key={id}>{render(id)}</React.Fragment>) : (
        <span className="flex items-center gap-2 text-xs text-text-muted">{emptyIcon} None</span>
      )}
      {ids.length > 7 ? <Badge variant="secondary">+{ids.length - 7}</Badge> : null}
    </div>
  </div>
);

function createID(): string {
  return crypto.randomUUID?.() ?? `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

function uniqueCopyName(name: string, presets: AppAttackPreset[]): string {
  const existing = new Set(presets.map((preset) => preset.name.toLowerCase()));
  let candidate = `${name} copy`;
  let suffix = 2;
  while (existing.has(candidate.toLowerCase())) candidate = `${name} copy ${suffix++}`;
  return candidate;
}

function cloneWaves(waves: AttackSetupDraft['waves']): AttackSetupDraft['waves'] {
  return waves.map((wave) => ({
    L: { troops: wave.L.troops.map((slot) => ({ ...slot })), tools: wave.L.tools.map((slot) => ({ ...slot })) },
    M: { troops: wave.M.troops.map((slot) => ({ ...slot })), tools: wave.M.tools.map((slot) => ({ ...slot })) },
    R: { troops: wave.R.troops.map((slot) => ({ ...slot })), tools: wave.R.tools.map((slot) => ({ ...slot })) },
  }));
}

function formatUpdatedAt(value: string): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime()) || date.getTime() === 0) return 'previously';
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

export default AttackPresetsView;
