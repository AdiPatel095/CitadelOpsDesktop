import { type ReactNode } from 'react';
import './Card.css';

export interface SectionHeaderProps {
  title: ReactNode;
  level?: 2 | 3;
  count?: ReactNode;
  actions?: ReactNode;
}
export const SectionHeader = ({ title, level = 2, count, actions }: SectionHeaderProps) => {
  const Heading = level === 2 ? 'h2' : 'h3';
  return (
    <div className="ui-section-header">
      <Heading className="ui-section-header__title">{title}</Heading>
      {count !== undefined && <span className="ui-section-header__count">{count}</span>}
      <span className="ui-section-header__rule" aria-hidden="true" />
      {actions && <div className="ui-section-header__actions">{actions}</div>}
    </div>
  );
};
