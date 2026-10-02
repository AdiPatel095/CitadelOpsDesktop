import type { HTMLAttributes, ReactNode } from 'react';
import './ViewState.css';
export interface EmptyStateProps extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
  title: ReactNode; description?: ReactNode; icon?: ReactNode; action?: ReactNode;
  size?: 'sm' | 'md' | 'lg'; surface?: 'plain' | 'outlined';
}
export function EmptyState({ title, description, icon, action, size = 'md', surface = 'plain', className = '', ...props }: EmptyStateProps) {
  return <div className={`ui-empty-state ui-state--${size} ui-empty-state--${surface} ${className}`} {...props}>
    {icon && <span className="ui-empty-state__icon" aria-hidden="true">{icon}</span>}
    <div className="ui-empty-state__title">{title}</div>
    {description && <div className="ui-empty-state__description">{description}</div>}
    {action && <div className="ui-empty-state__action">{action}</div>}
  </div>;
}
