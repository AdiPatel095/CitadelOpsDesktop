import React, { type HTMLAttributes } from 'react';
import './Card.css';

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  variant?: 'solid' | 'interactive';
}

export const Card = React.forwardRef<HTMLDivElement, CardProps>(
  ({ className = '', variant = 'solid', children, ...props }, ref) => (
    <div ref={ref} data-region="card" className={`ui-card ui-card--${variant} ${className}`} {...props}>{children}</div>
  )
);
Card.displayName = 'Card';

export interface CardHeaderProps extends HTMLAttributes<HTMLDivElement> {
  divided?: boolean;
}
export const CardHeader: React.FC<CardHeaderProps> = ({ divided = false, className = '', children, ...props }) => (
  <div className={`ui-card__header ${divided ? 'ui-card__header--divided' : ''} ${className}`} {...props}>{children}</div>
);
export const CardTitle: React.FC<HTMLAttributes<HTMLHeadingElement>> = ({ className = '', children, ...props }) => (
  <h3 className={`ui-card__title ${className}`} {...props}>{children}</h3>
);
export const CardDescription: React.FC<HTMLAttributes<HTMLParagraphElement>> = ({ className = '', children, ...props }) => (
  <p className={`ui-card__description ${className}`} {...props}>{children}</p>
);
export const CardActions: React.FC<HTMLAttributes<HTMLDivElement>> = ({ className = '', children, ...props }) => (
  <div className={`ui-card__actions ${className}`} {...props}>{children}</div>
);
export interface CardContentProps extends HTMLAttributes<HTMLDivElement> {
  flush?: boolean;
}
export const CardContent: React.FC<CardContentProps> = ({ flush = false, className = '', children, ...props }) => (
  <div className={`ui-card__content ${flush ? 'ui-card__content--flush' : ''} ${className}`} {...props}>{children}</div>
);
export const CardFooter: React.FC<HTMLAttributes<HTMLDivElement>> = ({ className = '', children, ...props }) => (
  <div className={`ui-card__footer ${className}`} {...props}>{children}</div>
);
