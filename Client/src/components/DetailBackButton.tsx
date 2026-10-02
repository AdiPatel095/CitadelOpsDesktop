import { ArrowLeft } from 'lucide-react';
import { Button } from './ui';

interface DetailBackButtonProps {
  label: string;
  onClick: () => void;
  className?: string;
}

const DetailBackButton = ({ label, onClick, className = '' }: DetailBackButtonProps) => (
  <Button
    variant="secondary"
    onClick={onClick}
    leftIcon={<ArrowLeft aria-hidden="true" />}
    className={`detail-back-button group ${className}`}
    aria-label={label}
  >
    <span>{label}</span>
  </Button>
);

export default DetailBackButton;
