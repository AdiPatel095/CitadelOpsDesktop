import React, { Suspense, useMemo, useState } from 'react';
import {
  ClipboardCopy,
  ClipboardPaste,
  Edit3,
  Files,
  Library,
  Plus,
  Shield,
  Swords,
  Trash2,
} from 'lucide-react';
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
  Modal,
  ModalTitle,
  PageHeader,
} from '../components/ui';
import { Notifications } from '../components/Notifications';
import ToolImage from '../components/ToolImage';
import UnitImage from '../components/UnitImage';
import {
  formatCRAAttackPresetString,
  parseCRAAttackPresetString,
} from '../attackPresets/AttackPresetCraCodec';
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
  const [importOpen, setImportOpen] = useState(false);
  const [importValue, setImportValue] = useState('');
  const [importError, setImportError] = useState('');

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

  const openImport = () => {
    setImportValue('');
    setImportError('');
    setImportOpen(true);
  };

  const handleImport = () => {
    try {
      const draft = parseCRAAttackPresetString(importValue);
      setImportOpen(false);
      setEditor({ presetID: null, draft });
      Notifications.success('CRA formation loaded. Review it and save the new preset.');
    } catch (error) {
      setImportError(errorMessage(error, 'Could not read the CRA command.'));
    }
  };

  const handleCopyShareString = async (preset: AppAttackPreset) => {
    try {
      await writeClipboardText(formatCRAAttackPresetString(preset));
      Notifications.success(`Copied “${preset.name}” as a formation-only CRA share string.`);
    } catch (error) {
      Notifications.error(errorMessage(error, 'Could not copy the CRA share string.'));
    }
  };

  return (
    <div className="mx-auto flex w-full max-w-[1800px] flex-col gap-5 pb-10">
      <PageHeader
        title="Attack Presets"
        description="Build reusable multi-wave formations, or import and share them as CRA command strings."
        icon={<Library className="h-5 w-5" />}
        actions={(
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="secondary" leftIcon={<ClipboardPaste className="h-4 w-4" />} onClick={openImport}>
              Import CRA
            </Button>
            <Button leftIcon={<Plus className="h-4 w-4" />} onClick={() => setEditor({ presetID: null })}>
              New preset
            </Button>
          </div>
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
              onCopyShare={() => void handleCopyShareString(preset)}
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

      <Modal
        isOpen={importOpen}
        onClose={() => setImportOpen(false)}
        maxWidth="xl"
        title={(
          <ModalTitle
            icon={<ClipboardPaste className="h-5 w-5" />}
            description="Accepts a full %xt% CRA wire command or its JSON payload."
          >
            Import CRA formation
          </ModalTitle>
        )}
        footer={(
          <div className="flex w-full items-center justify-end gap-2">
            <Button variant="ghost" onClick={() => setImportOpen(false)}>Cancel</Button>
            <Button
              onClick={handleImport}
              disabled={!importValue.trim()}
              leftIcon={<ClipboardPaste className="h-4 w-4" />}
            >
              Load formation
            </Button>
          </div>
        )}
      >
        <div className="space-y-4">
          <p className="text-sm leading-relaxed text-text-muted">
            Only the CRA <span className="font-mono text-text-main">A</span> wave formation is imported.
            Commander, source, target, travel, support, and account-specific fields are intentionally ignored.
          </p>
          <label className="grid gap-2 text-xs font-bold text-text-muted">
            CRA command or JSON payload
            <textarea
              value={importValue}
              onChange={(event) => {
                setImportValue(event.target.value);
                if (importError) setImportError('');
              }}
              rows={9}
              spellCheck={false}
              placeholder={'%xt%EmpireEx_21%cra%1%{"A":[...]}%'}
              className="w-full resize-y rounded-global border border-border-base bg-bg-input/70 px-4 py-3 font-mono text-xs font-normal text-text-main shadow-inner outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
            />
          </label>
          {importError ? (
            <div className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error">
              {importError}
            </div>
          ) : null}
        </div>
      </Modal>
    </div>
  );
};

const PresetCard: React.FC<{
  preset: AppAttackPreset;
  busy: boolean;
  onEdit: () => void;
  onCopyShare: () => void;
  onDuplicate: () => void;
  onDelete: () => void;
}> = ({ preset, busy, onEdit, onCopyShare, onDuplicate, onDelete }) => {
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
          <Button variant="ghost" size="icon" disabled={busy} onClick={onCopyShare} title="Copy CRA share string"><ClipboardCopy className="h-4 w-4" /></Button>
          <Button variant="ghost" size="icon" disabled={busy} onClick={onDuplicate} title="Duplicate preset"><Files className="h-4 w-4" /></Button>
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

async function writeClipboardText(value: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch {
      // Fall through when the desktop browser denies clipboard permission.
    }
  }

  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  const copied = document.execCommand('copy');
  textarea.remove();
  if (!copied) throw new Error('Clipboard access is unavailable.');
}

export default AttackPresetsView;
