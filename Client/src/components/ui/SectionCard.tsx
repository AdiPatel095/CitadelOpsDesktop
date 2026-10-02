import React, { useId, useState, type ReactNode } from 'react';
import { ChevronDown } from 'lucide-react';
import { Card, CardActions, CardContent, CardDescription, CardHeader, CardTitle, type CardProps } from './Card';

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
  collapsible?: boolean;
  expanded?: boolean;
  onExpandedChange?: (expanded: boolean) => void;
  toggleLabel?: string;
}

export const SectionCard: React.FC<SectionCardProps> = ({
  title, description, icon, actions, headerClassName = '', contentClassName = '',
  descriptionClassName = '', titleClassName = '', flush = false,
  collapsible = false, expanded, onExpandedChange, toggleLabel,
  variant = 'solid', className = '', children, ...props
}) => {
  const [internalExpanded, setInternalExpanded] = useState(true);
  const isExpanded = expanded ?? internalExpanded;
  const titleId = useId();
  const contentId = useId();
  const heading = (
    <div className="ui-card__heading">
      <CardTitle id={titleId} className={titleClassName}>
        {icon && <span className="ui-card__icon" aria-hidden="true">{icon}</span>}
        {title}
      </CardTitle>
      {description && <CardDescription className={descriptionClassName}>{description}</CardDescription>}
    </div>
  );
  return (
    <Card variant={variant} className={className} {...props}>
      <CardHeader className={headerClassName}>
        {collapsible ? (
          <button type="button" className="ui-card__disclosure" aria-expanded={isExpanded}
            aria-controls={contentId} aria-label={toggleLabel} aria-labelledby={toggleLabel ? undefined : titleId}
            onClick={() => {
              if (expanded === undefined) setInternalExpanded(!isExpanded);
              onExpandedChange?.(!isExpanded);
            }}>
            {heading}
            <ChevronDown className="ui-card__chevron" aria-hidden="true" />
          </button>
        ) : heading}
        {actions && <CardActions>{actions}</CardActions>}
      </CardHeader>
      <CardContent id={contentId} hidden={collapsible && !isExpanded} flush={flush} className={contentClassName}>
        {children}
      </CardContent>
    </Card>
  );
};
