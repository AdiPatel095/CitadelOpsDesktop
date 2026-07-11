import { useEffect, useMemo, useState } from 'react';
import { Braces, Play, ScanSearch } from 'lucide-react';
import { CitadelAPI } from '../api/CitadelClient';
import type { IntentDefinition, IntentReceipt } from '../api/Contracts';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle, Select } from './ui';

const EMPTY_ARGUMENTS = '{}';

const IntentConsole = () => {
  const [definitions, setDefinitions] = useState<IntentDefinition[]>([]);
  const [intentName, setIntentName] = useState('');
  const [argumentsText, setArgumentsText] = useState(EMPTY_ARGUMENTS);
  const [receipt, setReceipt] = useState<IntentReceipt | null>(null);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let active = true;
    void CitadelAPI.getIntentDefinitions()
      .then((items) => {
        if (!active) return;
        const sorted = [...items].sort((left, right) => left.name.localeCompare(right.name));
        setDefinitions(sorted);
        setIntentName((current) => current || sorted[0]?.name || '');
      })
      .catch((reason) => {
        if (active) setError(reason instanceof Error ? reason.message : 'Could not load intents.');
      });
    return () => { active = false; };
  }, []);

  const definition = useMemo(
    () => definitions.find((candidate) => candidate.name === intentName),
    [definitions, intentName],
  );

  const submit = async (dryRun: boolean) => {
    if (!intentName || submitting) return;
    let argumentsValue: Record<string, unknown>;
    try {
      const parsed = JSON.parse(argumentsText) as unknown;
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error('Arguments must be a JSON object.');
      }
      argumentsValue = parsed as Record<string, unknown>;
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Arguments are not valid JSON.');
      return;
    }
    setSubmitting(true);
    setError('');
    setReceipt(null);
    try {
      setReceipt(await CitadelAPI.submitIntent(intentName, argumentsValue, { actor: 'ui-intent-console', dryRun }));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Intent submission failed.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Card className="liquid-prominent-header-card mb-6 w-full">
      <CardHeader className="liquid-card-header-prominent">
        <div className="flex items-center gap-3">
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500/10 text-indigo-400">
            <Braces className="h-4 w-4" />
          </span>
          <div>
            <CardTitle className="text-lg">Intent Console</CardTitle>
            <p className="mt-1 text-xs font-semibold text-text-muted">Inspect or submit the same deterministic operations used by the UI and CLI.</p>
          </div>
        </div>
      </CardHeader>
      <CardContent className="liquid-prominent-header-content space-y-4 p-6">
        <Select
          value={intentName}
          onChange={(value) => { setIntentName(value); setReceipt(null); setError(''); }}
          options={definitions.map((item) => ({ value: item.name, label: item.name }))}
          placeholder="Select an intent"
          menuGrowToViewport
        />
        {definition && (
          <div className="flex flex-wrap items-center gap-2 text-xs text-text-muted">
            <Badge variant={definition.effect === 'read' ? 'secondary' : definition.effect === 'launch' ? 'danger' : 'warning'}>{definition.effect}</Badge>
            <span>{definition.description}</span>
          </div>
        )}
        <label className="grid gap-2 text-xs font-bold text-text-muted">
          Arguments JSON
          <textarea
            value={argumentsText}
            onChange={(event) => setArgumentsText(event.target.value)}
            rows={7}
            spellCheck={false}
            className="w-full rounded-global border border-border-base bg-bg-input/70 px-4 py-3 font-mono text-sm font-normal text-text-main shadow-inner outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
          />
        </label>
        <div className="flex flex-wrap gap-3">
          <Button variant="outline" onClick={() => void submit(true)} disabled={!intentName || submitting} leftIcon={<ScanSearch className="h-4 w-4" />}>
            Preview plan
          </Button>
          <Button variant="primary" onClick={() => void submit(false)} disabled={!intentName || submitting} leftIcon={<Play className="h-4 w-4" />}>
            Submit intent
          </Button>
        </div>
        {error && <div className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error">{error}</div>}
        {receipt && (
          <pre className="max-h-80 overflow-auto rounded-global border border-border-base bg-bg-app/60 p-4 text-xs text-text-main custom-scrollbar">
            {JSON.stringify(receipt, null, 2)}
          </pre>
        )}
      </CardContent>
    </Card>
  );
};

export default IntentConsole;
