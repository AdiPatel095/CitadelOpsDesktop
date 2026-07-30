import React, { type ReactNode } from 'react';
import { Card, CardContent, CardHeader, CardTitle, type CardProps } from './Card';

export interface SectionCardProps extends Omit<CardProps, 'title'> {
  title: ReactNode;
  description?: ReactNode;
  icon?: ReactNode;
  actions?: ReactNode;
  headerClassName?: string;
  contentClassName?: string;
  descriptionClassName?: string;
  titleClassName?: string;
  flush?: boolean;
}

export const SectionCard: React.FC<SectionCardProps> = ({
  title,
  description,
  icon,
  actions,
  headerClassName = '',
  contentClassName = '',
  descriptionClassName = 'font-semibold',
  titleClassName = '',
  flush = false,
  variant = 'solid',
  className = '',
  children,
  ...props
}) => (
  <Card variant={variant} className={`liquid-prominent-header-card ${className}`} {...props}>
    <CardHeader className={`liquid-card-header-prominent ${headerClassName}`}>
      <div className="min-w-0">
        <CardTitle className={`flex items-center gap-2 ${titleClassName}`}>
          {icon && <span className="text-primary" aria-hidden="true">{icon}</span>}
          {title}
        </CardTitle>
        {description && <p className={`mt-1 text-xs text-text-muted ${descriptionClassName}`}>{description}</p>}
      </div>
      {actions}
    </CardHeader>
    <CardContent className={`liquid-prominent-header-content ${flush ? 'liquid-prominent-header-content-flush' : ''} ${contentClassName}`}>
      {children}
    </CardContent>
  </Card>
);
