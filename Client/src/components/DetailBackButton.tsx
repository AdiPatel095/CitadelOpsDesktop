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
    className={`detail-back-button group ${className}`}
    aria-label={label}
  >
    <span className="detail-back-button-icon" aria-hidden="true">
      <ArrowLeft className="h-3.5 w-3.5 transition-transform duration-200 group-hover:-translate-x-0.5" />
    </span>
    <span>{label}</span>
  </Button>
);

export default DetailBackButton;
