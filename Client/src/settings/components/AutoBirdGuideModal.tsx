import { FeatureGuideModal } from './FeatureGuideModal';
export function AutoBirdGuideModal({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) { return <FeatureGuideModal feature="autoBird" isOpen={isOpen} onClose={onClose} />; }
