import { FeatureGuideModal } from './FeatureGuideModal';
export function AutoTowerGuideModal({ isOpen, onClose, showAdvisor = true }: { isOpen: boolean; onClose: () => void; showAdvisor?: boolean }) { return <FeatureGuideModal feature="autoTower" isOpen={isOpen} onClose={onClose} showAdvisor={showAdvisor} />; }
