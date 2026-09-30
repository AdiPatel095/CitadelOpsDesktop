import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, LoaderCircle, RotateCcw } from 'lucide-react';
import { APIError } from '../api/CitadelClient';
import type { ConfigurationSnapshot } from '../api/Contracts';
import { useCitadelAPI } from '../api/ApiContext';
import { Button } from '../components/ui/Button';
import { LocalizedText } from '../i18n/LocalizedText';
import { useLocale } from '../i18n/LocaleContext';

export type ConfigurationSaveCondition =
  | { expectedValue: unknown }
  | { expectedRevision: number };

export interface ConfigurationDraftSnapshot {
  key: string;
  revision: number;
  sections: Record<string, unknown>;
  section: string;
  sectionWasPresent: boolean;
  configurationWideBaseline: boolean;
}

export function captureConfigurationDraft(
  configuration: ConfigurationSnapshot,
  section: string,
  key = section,
  configurationWideBaseline = false,
): ConfigurationDraftSnapshot {
  return {
    key,
    revision: configuration.revision,
    sections: cloneConfigurationSections(configuration.sections),
    section,
    sectionWasPresent: Object.prototype.hasOwnProperty.call(configuration.sections, section),
    configurationWideBaseline,
  };
}

export function configurationDraftSaveCondition(
  snapshot: ConfigurationDraftSnapshot,
  section = snapshot.section,
  configurationWideBaseline = snapshot.configurationWideBaseline,
): ConfigurationSaveCondition {
  const sectionWasPresent = section === snapshot.section
    ? snapshot.sectionWasPresent
    : Object.prototype.hasOwnProperty.call(snapshot.sections, section);
  return sectionWasPresent && !configurationWideBaseline
    ? { expectedValue: snapshot.sections[section] }
    : { expectedRevision: snapshot.revision };
}

export function advanceConfigurationDraftAfterSave(
  snapshot: ConfigurationDraftSnapshot,
  configuration: ConfigurationSnapshot,
): ConfigurationDraftSnapshot {
  return captureConfigurationDraft(
    configuration,
    snapshot.section,
    snapshot.key,
    snapshot.configurationWideBaseline,
  );
}

interface UseConfigurationDraftSessionOptions {
  isOpen: boolean;
  section: string;
  /** Changes only for an intentional editor-context switch while the modal remains mounted. */
  sessionKey?: string;
  /** Configuration sections whose open-time contents affect validation or the value being saved. */
  configurationDependencies?: readonly string[];
  /**
   * A castle copy was applied to this draft (CIT-21). The conflict notice then offers to load the latest settings and
   * re-apply the copy (`onReloaded` runs once the latest settings are loaded), or to load them and drop the copy
   * (`onDropped`).
   */
  copyReplay?: { onReloaded: () => void; onDropped: () => void };
}

export function useConfigurationDraftSession({
  isOpen,
  section,
  sessionKey = section,
  configurationDependencies = [],
  copyReplay,
}: UseConfigurationDraftSessionOptions) {
  const { loadLatestConfiguration, updateConfiguration } = useCitadelAPI();
  const { t: localizeStatic } = useLocale();
  const [snapshot, setSnapshot] = useState<ConfigurationDraftSnapshot | null>(null);
  const [initialSnapshot, setInitialSnapshot] = useState<ConfigurationDraftSnapshot | null>(null);
  const [openGeneration, setOpenGeneration] = useState(0);
  // Counts loads of the saved configuration only (not a recovered draft being restored), so draft recovery can tell
  // "the editor loaded" from "the player restored a draft".
  const [loadGeneration, setLoadGeneration] = useState(0);
  const [recoveredExtras, setRecoveredExtras] = useState<{ generation: number; value: unknown } | null>(null);
  const recoveredCount = useRef(0);
  const [conflict, setConflict] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState('');
  const snapshotRef = useRef<ConfigurationDraftSnapshot | null>(null);
  const activeSessionKey = useRef<string | null>(null);
  const loadRequest = useRef(0);
  const isOpenRef = useRef(isOpen);
  const sessionKeyRef = useRef(sessionKey);
  const hasConfigurationDependencies = configurationDependencies.length > 0;
  isOpenRef.current = isOpen;
  sessionKeyRef.current = sessionKey;

  const loadLatest = useCallback(async (): Promise<boolean> => {
    const request = ++loadRequest.current;
    const requestedSessionKey = sessionKey;
    setLoading(true);
    setLoadError('');
    try {
      const latest = await loadLatestConfiguration();
      if (request !== loadRequest.current
        || !isOpenRef.current
        || sessionKeyRef.current !== requestedSessionKey) return false;
      const captured = captureConfigurationDraft(
        latest,
        section,
        sessionKey,
        hasConfigurationDependencies,
      );
      snapshotRef.current = captured;
      setSnapshot(captured);
      setInitialSnapshot(captured);
      setOpenGeneration((current) => current + 1);
      setLoadGeneration((current) => current + 1);
      setRecoveredExtras(null);
      setConflict(false);
      return true;
    } catch (error) {
      if (request !== loadRequest.current
        || !isOpenRef.current
        || sessionKeyRef.current !== requestedSessionKey) return false;
      setLoadError(error instanceof Error ? error.message : localizeStatic('ui.settings.configurationDraftSession.could.not.load.the.latest.saved.settings.69428e73'));
      return false;
    } finally {
      if (request === loadRequest.current) setLoading(false);
    }
  }, [hasConfigurationDependencies, loadLatestConfiguration, localizeStatic, section, sessionKey]);

  useEffect(() => {
    if (!isOpen) {
      loadRequest.current += 1;
      activeSessionKey.current = null;
      snapshotRef.current = null;
      setSnapshot(null);
      setRecoveredExtras(null);
      setInitialSnapshot(null);
      setConflict(false);
      setLoading(false);
      setLoadError('');
      return;
    }
    if (activeSessionKey.current === sessionKey) return;
    activeSessionKey.current = sessionKey;
    snapshotRef.current = null;
    setSnapshot(null);
    setRecoveredExtras(null);
    setInitialSnapshot(null);
    setConflict(false);
    void loadLatest();
  }, [isOpen, loadLatest, sessionKey]);

  const saveSection = useCallback(async (
    targetSection: string,
    value: unknown,
    configurationWideBaseline = hasConfigurationDependencies,
  ) => {
    const baseline = snapshotRef.current;
    if (baseline == null) throw new Error(localizeStatic('ui.settings.configurationDraftSession.settings.are.still.loading.e887bade'));
    const request = loadRequest.current;
    try {
      const updated = await updateConfiguration(
        targetSection,
        value,
        configurationDraftSaveCondition(baseline, targetSection, configurationWideBaseline),
      );
      if (request !== loadRequest.current) return updated;
      const captured = advanceConfigurationDraftAfterSave(baseline, updated);
      snapshotRef.current = captured;
      setSnapshot(captured);
      setConflict(false);
      return updated;
    } catch (error) {
      if (request === loadRequest.current
        && error instanceof APIError
        && error.code === 'configuration_conflict') {
        setConflict(true);
      }
      throw error;
    }
  }, [hasConfigurationDependencies, localizeStatic, updateConfiguration]);

  const save = useCallback(
    (value: unknown) => saveSection(section, value),
    [saveSection, section],
  );

  const reloadLatest = useCallback(async () => {
    await loadLatest();
  }, [loadLatest]);

  // "Load latest and re-apply copy": the reviewed copy is applied again once the latest settings are loaded.
  const reloadAndReapplyCopy = useCallback(async () => {
    if (await loadLatest()) copyReplay?.onReloaded();
  }, [copyReplay, loadLatest]);
  // "Load latest settings": the copy is dropped.
  const reloadAndDropCopy = useCallback(async () => {
    copyReplay?.onDropped();
    await loadLatest();
  }, [copyReplay, loadLatest]);

  /**
   * Puts a recovered unsaved draft (CIT-19) into the editor: the editor's load effect reads it exactly as it reads a
   * saved value. Only the editor's baseline for loading changes; the saved snapshot that guards Save (`snapshotRef`)
   * is untouched, so Save still compares against what is really saved. Never saves and never touches
   * `automation.enabled`.
   */
  const recoverDraft = useCallback((targetSection: string, value: unknown, extras?: unknown) => {
    setInitialSnapshot((current) => (current == null ? current : {
      ...current,
      sections: { ...current.sections, [targetSection]: cloneConfigurationSections({ value }).value },
    }));
    setOpenGeneration((current) => current + 1);
    recoveredCount.current += 1;
    setRecoveredExtras(extras === undefined ? null : { generation: recoveredCount.current, value: extras });
  }, []);

  const conflictNotice = useMemo(() => {
    if (loading && snapshot == null) {
      return (
        <div className="mb-4 flex items-center gap-2 rounded-global border border-border-base bg-bg-card/70 px-4 py-3 text-sm font-semibold text-text-muted" role="status">
          <LoaderCircle className="h-4 w-4 animate-spin" /> <LocalizedText messageKey="ui.settings.configurationDraftSession.loading.latest.saved.settings.884d319c" />
        </div>
      );
    }
    if (loadError) {
      return (
        <div className="mb-4 rounded-global border border-error/30 bg-error/10 px-4 py-3" role="alert">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div className="text-sm font-black text-error"><LocalizedText messageKey="ui.settings.configurationDraftSession.could.not.load.latest.settings.31ab2414" /></div>
              <p className="mt-1 text-xs text-text-main">{loadError}</p>
            </div>
            <Button variant="outline" size="sm" onClick={() => void reloadLatest()} leftIcon={<RotateCcw className="h-4 w-4" />}>
              <LocalizedText messageKey="ui.settings.configurationDraftSession.retry.942087cc" />
            </Button>
          </div>
        </div>
      );
    }
    if (!conflict) return null;
    const copied = copyReplay !== undefined;
    return (
      <div className="mb-4 rounded-global border border-warning/40 bg-warning/10 px-4 py-3" role="alert">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-sm font-black text-warning">
              <AlertTriangle className="h-4 w-4 shrink-0" /> <LocalizedText messageKey="ui.settings.configurationDraftSession.settings.changed.elsewhere.0e978d4f" />
            </div>
            <p className="mt-1 text-xs leading-relaxed text-text-main">
              {copied
                ? <LocalizedText messageKey="castleCopy.conflictNotice" />
                : <LocalizedText messageKey="ui.settings.configurationDraftSession.your.unsaved.draft.is.still.here.review.0e9b1deb" />}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            {copied ? (
              <Button
                variant="primary"
                size="sm"
                disabled={loading}
                onClick={() => void reloadAndReapplyCopy()}
                leftIcon={<RotateCcw className={`h-4 w-4${loading ? ' animate-spin' : ''}`} />}
              >
                <LocalizedText messageKey="castleCopy.loadAndReapply" />
              </Button>
            ) : null}
            <Button
              variant="outline"
              size="sm"
              disabled={loading}
              onClick={() => void (copied ? reloadAndDropCopy() : reloadLatest())}
              leftIcon={copied ? undefined : <RotateCcw className={`h-4 w-4${loading ? ' animate-spin' : ''}`} />}
            >
              {loading
                ? <LocalizedText messageKey="ui.settings.configurationDraftSession.loading.ba3bbbe1" />
                : copied
                  ? <LocalizedText messageKey="castleCopy.loadLatestOnly" />
                  : <LocalizedText messageKey="ui.settings.configurationDraftSession.load.latest.86edc870" />}
            </Button>
          </div>
        </div>
      </div>
    );
  }, [conflict, copyReplay, loadError, loading, reloadAndDropCopy, reloadAndReapplyCopy, reloadLatest, snapshot]);

  return {
    snapshot,
    initialSnapshot,
    sections: snapshot?.sections,
    initialSections: initialSnapshot?.sections,
    openKey: snapshot == null ? '' : `${snapshot.key}:${openGeneration}`,
    /** Changes only when the saved configuration is (re)loaded, never for a restored draft. */
    loadKey: snapshot == null ? '' : `${snapshot.key}:${loadGeneration}`,
    recoverDraft,
    recoveredExtras,
    save,
    saveSection,
    conflict,
    loading,
    ready: snapshot != null && !loading,
    conflictNotice,
    reloadLatest,
  };
}

function cloneConfigurationSections(sections: Record<string, unknown>): Record<string, unknown> {
  if (typeof structuredClone === 'function') return structuredClone(sections);
  return JSON.parse(JSON.stringify(sections)) as Record<string, unknown>;
}
