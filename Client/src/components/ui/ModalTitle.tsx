import React, { type ReactNode } from 'react';

export interface ModalTitleProps {
  children: ReactNode;
  icon: ReactNode;
  description?: ReactNode;
  trailing?: ReactNode;
  className?: string;
}

export const ModalTitle: React.FC<ModalTitleProps> = ({
  children,
  icon,
  description,
  trailing,
  className = '',
}) => (
  <div className={`scheduler-modal-title ${className}`}>
    <span className="scheduler-modal-title-mark" aria-hidden="true">{icon}</span>
    <span className="flex min-w-0 flex-col">
      <span className="scheduler-modal-title-text">{children}</span>
      {description && <span className="ui-modal-title-description">{description}</span>}
    </span>
    {trailing}
  </div>
);
