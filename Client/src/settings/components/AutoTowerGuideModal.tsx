import { useEffect, useState } from 'react';
import { Button, Modal } from '../../components/ui';
import guideData from '../../config/autoTowerGuide.json';

const guide = guideData.steps;

type GuideImage = { src: string; alt: string; caption: string; width: number; height: number };

export function AutoTowerGuideModal({ isOpen, onClose, showAdvisor = true }: {
  isOpen: boolean;
  onClose: () => void;
  showAdvisor?: boolean;
}) {
  const [preview, setPreview] = useState<GuideImage | null>(null);
  useEffect(() => { if (!isOpen) setPreview(null); }, [isOpen]);
  const closeGuide = () => { setPreview(null); onClose(); };
  return (
    <>
      <Modal isOpen={isOpen} onClose={closeGuide} title="Auto Towers guide" maxWidth="3xl"
        footer={<Button variant="outline" onClick={closeGuide}>Back to settings</Button>}>
        <p className="mb-4 text-sm text-text-muted">Follow these steps to set up Auto Towers; select a fictional example picture to enlarge it.</p>
        <p className="mb-4 text-sm text-text-muted">{guideData.recommendationIntro}</p>
        <ol className="space-y-6">
          {guide.filter((step) => showAdvisor || step.id !== 'advisor').map((step, index) => (
            <li key={step.id}>
              <h3 className="mb-2 text-sm font-bold text-primary">{index + 1}. {step.title}</h3>
              <dl className="space-y-3">
                {step.items.map((item) => (
                  <div key={item.label}>
                    <dt className="text-sm font-semibold text-text-main">{item.label}</dt>
                    <dd className="mt-1 text-sm leading-relaxed text-text-muted">{item.description}</dd>
                    {'recommendation' in item && typeof item.recommendation === 'string' && <dd className="mt-1 text-sm leading-relaxed text-text-main"><strong>Recommended start:</strong> {item.recommendation}</dd>}
                  </div>
                ))}
              </dl>
              {step.image && <figure className="mt-3" style={{ maxWidth: step.image.width }}>
                <button type="button" onClick={() => setPreview(step.image)} aria-label={`Enlarge ${step.title.toLowerCase()} picture`}
                  className="block w-full overflow-hidden rounded-xl border border-border-base hover:border-primary focus-visible:outline-2 focus-visible:outline-primary">
                  <img src={step.image.src} alt={step.image.alt} width={step.image.width} height={step.image.height} loading="lazy" className="h-auto w-full" />
                </button>
                <figcaption className="mt-2 text-xs leading-relaxed text-text-muted">{step.image.caption}</figcaption>
              </figure>}
            </li>
          ))}
        </ol>
      </Modal>
      <Modal isOpen={isOpen && preview !== null} onClose={() => setPreview(null)} title="Auto Towers setup example" maxWidth="full"
        footer={<Button variant="outline" onClick={() => setPreview(null)}>Back to guide</Button>}>
        {preview && <div className="overflow-auto" tabIndex={0} role="region" aria-label="Full-size picture; scroll to view all settings"><img src={preview.src} alt={preview.alt} width={preview.width} height={preview.height} className="h-auto max-w-none" style={{ width: preview.width }} /></div>}
      </Modal>
    </>
  );
}
