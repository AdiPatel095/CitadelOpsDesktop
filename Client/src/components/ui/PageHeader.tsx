import React, { type HTMLAttributes, type ReactNode } from 'react';

export interface PageHeaderProps extends Omit<HTMLAttributes<HTMLElement>, 'title'> {
  title: ReactNode;
  description?: ReactNode;
  eyebrow?: ReactNode;
  icon?: ReactNode;
  actions?: ReactNode;
  meta?: ReactNode;
}

export const PageHeader: React.FC<PageHeaderProps> = ({
  title,
  description,
  eyebrow = 'Command center',
  icon,
  actions,
  meta,
  className = '',
  ...props
}) => (
  <header className={`m3-page-header ui-page-header ${className}`} {...props}>
    <span className="m3-page-header-shape" aria-hidden="true" />
    <div className="ui-page-header-heading">
      {icon && <span className="ui-page-header-icon" aria-hidden="true">{icon}</span>}
      <div className="min-w-0">
        {eyebrow && <div className="ui-page-header-eyebrow">{eyebrow}</div>}
        <h1 className="ui-page-header-title">{title}</h1>
        {description && <p className="ui-page-header-description">{description}</p>}
      </div>
    </div>
    {(actions || meta) && (
      <div className="ui-page-header-aside">
        {meta}
        {actions}
      </div>
    )}
  </header>
);
