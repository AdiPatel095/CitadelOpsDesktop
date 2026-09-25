import { useLocale } from '../i18n/LocaleContext';
import { useLocalizedErrorState } from '../i18n/useLocalizedErrorState';
import { useLocalizedMessage } from '../i18n/useLocalizedMessage';
import { parseMessageDescriptor } from '../i18n/messageDescriptor';
import { LocalizedError } from '../i18n/LocalizedError';
import { useEffect, useMemo, useState } from 'react';
import { Braces, Play, ScanSearch } from 'lucide-react';
import { CitadelAPI } from '../api/CitadelClient';
import type { IntentDefinition, IntentReceipt } from '../api/Contracts';
import { Badge, Button, SectionCard, Select } from './ui';

const EMPTY_ARGUMENTS = '{}';

const IntentConsole = () => {
  const {t,messageLocale} = useLocale();
  const [definitions, setDefinitions] = useState<IntentDefinition[]>([]);
  const [intentName, setIntentName] = useState('');
  const [argumentsText, setArgumentsText] = useState(EMPTY_ARGUMENTS);
  const [receipt, setReceipt] = useState<IntentReceipt | null>(null);
  const [error, setError, errorMessage] = useLocalizedErrorState();
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
        if (active) setError(reason instanceof Error ? reason : new LocalizedError('intent.loadFailed'));
      });
    return () => { active = false; };
  }, []);

  const definition = useMemo(
    () => definitions.find((candidate) => candidate.name === intentName),
    [definitions, intentName],
  );

  const descriptionDescriptor = useMemo(()=>parseMessageDescriptor(definition?.descriptionDescriptor),[definition]);
  const description = useLocalizedMessage(descriptionDescriptor,definition?.description ?? '');

	const selectIntent = (name: string) => {
		setIntentName(name);
		const next = definitions.find((candidate) => candidate.name === name);
		setArgumentsText(next?.argumentsExample ? JSON.stringify(next.argumentsExample, null, 2) : EMPTY_ARGUMENTS);
		setReceipt(null);
		setError('');
	};

  const submit = async (dryRun: boolean) => {
    if (!intentName || submitting) return;
    let argumentsValue: Record<string, unknown>;
    try {
      const parsed = JSON.parse(argumentsText) as unknown;
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new LocalizedError('intent.jsonObject');
      }
      argumentsValue = parsed as Record<string, unknown>;
    } catch (reason) {
      setError(reason instanceof LocalizedError ? reason : new LocalizedError('intent.jsonInvalid',undefined,reason));
      return;
    }
    setSubmitting(true);
    setError('');
    setReceipt(null);
    try {
      setReceipt(await CitadelAPI.submitIntent(intentName, argumentsValue, { actor: 'ui-intent-console', dryRun }));
    } catch (reason) {
      setError(reason instanceof Error ? reason : new LocalizedError('intent.submitFailed'));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div lang={messageLocale}><SectionCard variant="solid" className="mb-6 w-full" title={t('intent.title')}
      icon={<span className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500/10 text-indigo-400"><Braces className="h-4 w-4" /></span>}
      description={t('intent.description')} contentClassName="space-y-4 p-6">
        <Select
          value={intentName}
          onChange={selectIntent}
          options={definitions.map((item) => ({ value: item.name, label: item.name }))}
          placeholder={t('intent.select')}
          menuGrowToViewport
        />
        {definition && (
          <div className="flex flex-wrap items-center gap-2 text-xs text-text-muted">
            <Badge variant={definition.effect === 'read' ? 'secondary' : definition.effect === 'launch' ? 'danger' : 'warning'}>{['read','write','launch','external'].includes(definition.effect) ? t(`intent.effect.${definition.effect}`) : definition.effect}</Badge>
            <span lang={description.resolvedLocale === 'mixed' ? undefined : description.resolvedLocale}>{description.text}</span>
          </div>
        )}
        <label className="grid gap-2 text-xs font-bold text-text-muted">
          {t('intent.arguments')}
          <textarea dir="ltr" lang="en"
            value={argumentsText}
            onChange={(event) => setArgumentsText(event.target.value)}
            rows={7}
            spellCheck={false}
            className="w-full rounded-global border border-border-base bg-bg-input/70 px-4 py-3 font-mono text-sm font-normal text-text-main shadow-inner outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
          />
        </label>
        <div className="flex flex-wrap gap-3">
          <Button variant="outline" onClick={() => void submit(true)} disabled={!intentName || submitting} leftIcon={<ScanSearch className="h-4 w-4" />}>
            {t('intent.preview')}
          </Button>
          <Button variant="primary" onClick={() => void submit(false)} disabled={!intentName || submitting} leftIcon={<Play className="h-4 w-4" />}>
            {t('intent.submit')}
          </Button>
        </div>
        {error && <div lang={errorMessage.resolvedLocale === 'mixed' ? undefined : errorMessage.resolvedLocale} className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error">{error}</div>}
        {receipt && (
          <pre dir="ltr" lang="en" className="max-h-80 overflow-auto rounded-global border border-border-base bg-bg-app/60 p-4 text-xs text-text-main custom-scrollbar">
            {JSON.stringify(receipt, null, 2)}
          </pre>
        )}
    </SectionCard></div>
  );
};

export default IntentConsole;
