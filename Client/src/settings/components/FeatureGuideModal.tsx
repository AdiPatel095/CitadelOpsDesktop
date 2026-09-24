import { useEffect, useState } from 'react';
import { Button, Modal } from '../../components/ui';
import towerSource from '../../config/autoTowerGuide.json';
import birdSource from '../../config/autoBirdGuide.json';
import stationSource from '../../config/autoStationGuide.json';
import fortressSource from '../../config/autoFortressGuide.json';
import invasionSource from '../../config/autoInvasionGuide.json';
import nomadSource from '../../config/autoNomadGuide.json';
import khanSource from '../../config/autoKhanGuide.json';
import beriSource from '../../config/autoBeriGuide.json';
import advisorSource from '../../config/autoAdvisorGuide.json';
import { englishGuidePack, useGuideLocale } from '../../config/useGuideLocale';
import { GuideIllustration, type GuidePanelKind } from '../../config/GuideIllustration';

type Feature = 'autoTower' | 'autoBird' | 'autoStation' | 'autoFortress' | 'autoInvasion' | 'autoNomad' | 'autoAdvisor' | 'autoKhan' | 'autoBeri';
type TranslatedGuide = { recommendationIntro: string; steps: Record<string, { title: string; items: Record<string, { label: string; description: string; recommendation?: string }>; image?: { alt: string; caption: string } }> };
type SourceStep = { id: string; title: string; items: Array<{ id: string; label: string; description: string; recommendation?: string }>; image?: { src: string; alt: string; caption: string; width: number; height: number } };
const panelKind = (feature: Feature, step: string): GuidePanelKind => feature === 'autoBeri' ? ('beri' + step[0].toUpperCase() + step.slice(1)) as GuidePanelKind : feature === 'autoTower' ? (step === 'general' ? 'towerGeneral' : 'towerCastle') : feature === 'autoBird' ? (step === 'general' ? 'birdGeneral' : 'birdCastle') : feature === 'autoStation' ? 'stationGeneral' : feature === 'autoFortress' ? (step === 'kingdoms' ? 'fortressKingdoms' : step === 'supply' ? 'fortressSupply' : 'fortressAttack') : feature === 'autoInvasion' ? (step === 'open' ? 'invasionSetup' : step === 'difficulty' ? 'invasionDifficulty' : step === 'limits' ? 'invasionLimits' : 'invasionFortify') : feature === 'autoNomad' ? (step === 'open' ? 'nomadSetup' : step === 'difficulty' ? 'nomadDifficulty' : step === 'limits' ? 'nomadLimits' : step === 'cooldowns' ? 'nomadCooldowns' : 'nomadTrial') : feature === 'autoAdvisor' ? (step === 'open' ? 'advisorSetup' : step === 'difficulty' ? 'advisorDifficulty' : step === 'sizing' ? 'advisorSizing' : step === 'resources' ? 'advisorResources' : step === 'activation' ? 'advisorActivation' : 'advisorOverview') : step === 'setup' ? 'khanSetup' : step === 'rage' ? 'khanRage' : step === 'limits' ? 'khanLimits' : step === 'cooldowns' ? 'khanCooldowns' : step === 'protection' ? 'khanProtection' : 'khanSave';

export function FeatureGuideModal({ feature, isOpen, onClose, showAdvisor = true }: { feature: Feature; isOpen: boolean; onClose: () => void; showAdvisor?: boolean }) {
  const { locale: selectedLocale, pack: selectedPack } = useGuideLocale();
  const pack = (feature === 'autoFortress' && !('autoFortress' in selectedPack) || feature === 'autoInvasion' && !('autoInvasion' in selectedPack) || feature === 'autoNomad' && !('autoNomad' in selectedPack) || feature === 'autoAdvisor' && !('autoAdvisor' in selectedPack) || feature === 'autoKhan' && !('autoKhan' in selectedPack) || feature === 'autoBeri' && !('autoBeri' in selectedPack)) ? englishGuidePack : selectedPack;
  const locale = pack === englishGuidePack ? 'en' : selectedLocale;
  const [previewStepId, setPreviewStepId] = useState<string | null>(null);
  useEffect(() => { if (!isOpen) setPreviewStepId(null); }, [isOpen]);
  const source = feature === 'autoTower' ? towerSource : feature === 'autoBird' ? birdSource : feature === 'autoStation' ? stationSource : feature === 'autoFortress' ? fortressSource : feature === 'autoInvasion' ? invasionSource : feature === 'autoNomad' ? nomadSource : feature === 'autoAdvisor' ? advisorSource : feature === 'autoBeri' ? beriSource : khanSource;
  const guide = pack[feature] as unknown as TranslatedGuide;
  const steps = (source.steps as SourceStep[]).filter((step) => showAdvisor || step.id !== 'advisor');
  const previewStep = steps.find((step) => step.id === previewStepId && step.image);
  const closeGuide = () => { setPreviewStepId(null); onClose(); };
  const title = feature === 'autoTower' ? pack.ui.towerGuideTitle : feature === 'autoBird' ? pack.ui.birdGuideTitle : feature === 'autoStation' ? pack.ui.stationGuideTitle : feature === 'autoFortress' ? pack.ui.fortressGuideTitle : feature === 'autoInvasion' ? pack.ui.invasionGuideTitle : feature === 'autoNomad' ? pack.ui.nomadGuideTitle : feature === 'autoAdvisor' ? pack.ui.advisorGuideTitle : feature === 'autoBeri' ? pack.ui.beriGuideTitle : pack.ui.khanGuideTitle;
  const intro = feature === 'autoTower' ? pack.ui.towerGuideIntro : feature === 'autoBird' ? pack.ui.birdGuideIntro : feature === 'autoStation' ? pack.ui.stationGuideIntro : feature === 'autoFortress' ? pack.ui.fortressGuideIntro : feature === 'autoInvasion' ? pack.ui.invasionGuideIntro : feature === 'autoNomad' ? pack.ui.nomadGuideIntro : feature === 'autoAdvisor' ? pack.ui.advisorGuideIntro : feature === 'autoBeri' ? pack.ui.beriGuideIntro : pack.ui.khanGuideIntro;
  const previewTitle = feature === 'autoTower' ? pack.ui.towerPreviewTitle : feature === 'autoBird' ? pack.ui.birdPreviewTitle : feature === 'autoStation' ? pack.ui.stationPreviewTitle : feature === 'autoFortress' ? pack.ui.fortressPreviewTitle : feature === 'autoInvasion' ? pack.ui.invasionPreviewTitle : feature === 'autoNomad' ? pack.ui.nomadPreviewTitle : feature === 'autoAdvisor' ? pack.ui.advisorPreviewTitle : feature === 'autoBeri' ? pack.ui.beriPreviewTitle : pack.ui.khanPreviewTitle;
  return <>
    <Modal isOpen={isOpen} onClose={closeGuide} title={title} contentLang={locale} contentDir={locale === 'ar' ? 'rtl' : 'ltr'} closeLabel={pack.ui.backToSettings} maxWidth="3xl"
      footer={<Button variant="outline" onClick={closeGuide}>{pack.ui.backToSettings}</Button>}>
      <div lang={locale} dir={locale === 'ar' ? 'rtl' : 'ltr'}>
        <p className="mb-4 text-sm text-text-muted">{intro}</p>
        <p className="mb-4 text-sm text-text-muted">{guide.recommendationIntro}</p>
        <ol className="space-y-6">
          {steps.map((step, index) => {
            const content = guide.steps[step.id as keyof typeof guide.steps];
            return <li key={step.id}>
              <h3 className="mb-2 text-sm font-bold text-primary">{index + 1}. {content.title}</h3>
              <dl className="space-y-3">
                {step.items.map((item) => {
                  const translated = (content.items as Record<string, { label: string; description: string; recommendation?: string }>)[item.id];
                  return <div key={item.id}>
                    <dt className="text-sm font-semibold text-text-main">{translated.label}</dt>
                    <dd className="mt-1 text-sm leading-relaxed text-text-muted">{translated.description}</dd>
                    {'recommendation' in translated && typeof translated.recommendation === 'string' && <dd className="mt-1 text-sm leading-relaxed text-text-main"><strong>{pack.ui.recommendedLabel}:</strong> {translated.recommendation}</dd>}
                  </div>;
                })}
              </dl>
              {step.image && 'image' in content && content.image && <figure className="mt-3" style={{ maxWidth: locale === 'en' && feature !== 'autoFortress' && feature !== 'autoInvasion' && feature !== 'autoNomad' && feature !== 'autoAdvisor' && feature !== 'autoKhan' && feature !== 'autoBeri' ? step.image.width : 680 }}>
                <button type="button" onClick={() => setPreviewStepId(step.id)} aria-label={pack.ui.enlargePicture}
                  className="block w-full overflow-hidden rounded-xl border border-border-base hover:border-primary focus-visible:outline-2 focus-visible:outline-primary">
                  {locale === 'en' && feature !== 'autoFortress' && feature !== 'autoInvasion' && feature !== 'autoNomad' && feature !== 'autoAdvisor' && feature !== 'autoKhan' && feature !== 'autoBeri' ? <img src={step.image.src} alt={content.image.alt} width={step.image.width} height={step.image.height} loading="lazy" className="h-auto w-full" /> :
                    <GuideIllustration pack={pack} kind={panelKind(feature, step.id)} locale={locale} alt={[pack.ui.illustrativeExample, content.title, pack.panels[panelKind(feature, step.id)].title].join(' — ')} showAdvisor={showAdvisor} />}
                </button>
                <figcaption className="mt-2 text-xs leading-relaxed text-text-muted">{(locale !== 'en' || feature === 'autoFortress' || feature === 'autoInvasion' || feature === 'autoNomad' || feature === 'autoAdvisor' || feature === 'autoKhan' || feature === 'autoBeri') && <strong>{pack.ui.illustrativeExample}. </strong>}{content.image.caption}</figcaption>
              </figure>}
            </li>;
          })}
        </ol>
      </div>
    </Modal>
    <Modal isOpen={isOpen && !!previewStep} onClose={() => setPreviewStepId(null)} title={previewTitle} contentLang={locale} contentDir={locale === 'ar' ? 'rtl' : 'ltr'} closeLabel={pack.ui.backToGuide} maxWidth="full"
      footer={<Button variant="outline" onClick={() => setPreviewStepId(null)}>{pack.ui.backToGuide}</Button>}>
      {previewStep && (() => {
        const content = guide.steps[previewStep.id as keyof typeof guide.steps];
        if (!previewStep.image || !('image' in content) || !content.image) return null;
        return <div lang={locale} dir={locale === 'ar' ? 'rtl' : 'ltr'} className="overflow-auto" tabIndex={0} role="region" aria-label={pack.ui.fullSizePicture}>
          {locale === 'en' && feature !== 'autoFortress' && feature !== 'autoInvasion' && feature !== 'autoNomad' && feature !== 'autoAdvisor' && feature !== 'autoKhan' && feature !== 'autoBeri' ? <img src={previewStep.image.src} alt={content.image.alt} width={previewStep.image.width} height={previewStep.image.height} className="h-auto max-w-none" style={{ width: previewStep.image.width }} /> :
            <GuideIllustration pack={pack} kind={panelKind(feature, previewStep.id)} locale={locale} large alt={[pack.ui.illustrativeExample, content.title, pack.panels[panelKind(feature, previewStep.id)].title].join(' — ')} showAdvisor={showAdvisor} />}
        </div>;
      })()}
    </Modal>
  </>;
}
