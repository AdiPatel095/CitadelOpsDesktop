import { Button, Modal } from '../../components/ui';
import guide from '../../config/autoTowerGuide.json';

export function AutoTowerGuideModal({ isOpen, onClose, showAdvisor = true }: {
  isOpen: boolean;
  onClose: () => void;
  showAdvisor?: boolean;
}) {
  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Auto Towers guide" maxWidth="3xl"
      footer={<Button variant="outline" onClick={onClose}>Back to settings</Button>}>
      <div className="space-y-5">
        {guide.filter((group) => showAdvisor || group.id !== 'advisor').map((group) => (
          <section key={group.id}>
            <h3 className="mb-2 text-sm font-bold text-primary">{group.title}</h3>
            <dl className="space-y-3">
              {group.items.map((item) => (
                <div key={item.label}>
                  <dt className="text-sm font-semibold text-text-main">{item.label}</dt>
                  <dd className="mt-1 text-sm leading-relaxed text-text-muted">{item.description}</dd>
                </div>
              ))}
            </dl>
          </section>
        ))}
      </div>
    </Modal>
  );
}
