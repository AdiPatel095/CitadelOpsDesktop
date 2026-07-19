import React, { type HTMLAttributes, type ReactNode } from 'react';

export interface PageHeaderProps extends Omit<HTMLAttributes<HTMLElement>, 'title'> {
  title: ReactNode;
  description?: ReactNode;
  icon?: ReactNode;
  actions?: ReactNode;
  meta?: ReactNode;
}

export const PageHeader: React.FC<PageHeaderProps> = ({
  title,
  description,
  icon,
  actions,
  meta,
  className = '',
  ...props
}) => (
  <header className={`ui-page-header ${className}`} {...props}>
    <div className="ui-page-header-heading">
      {icon && <span className="ui-page-header-icon" aria-hidden="true">{icon}</span>}
      <div className="min-w-0">
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
