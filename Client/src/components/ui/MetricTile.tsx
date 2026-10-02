import { useLocale } from '../../i18n/LocaleContext';
import { type HTMLAttributes, type ReactNode } from 'react';
import './Card.css';

export interface MetricTileProps extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
  label: ReactNode;
  value: ReactNode;
  tone?: 'default' | 'brand' | 'success' | 'warning' | 'danger' | 'info';
  size?: 'sm' | 'md' | 'lg';
  monospace?: boolean;
  caption?: ReactNode;
}
export const MetricTile = ({ label, value, tone = 'default', size = 'md', monospace = true,
  caption, className = '', ...props }: MetricTileProps) => {
  const { number, locale } = useLocale();
  return (
    <div className={`ui-metric-tile ${className}`} data-tone={tone} data-size={size} {...props}>
      <div className="ui-metric-tile__label">{label}</div>
      <div className={`ui-metric-value ${monospace ? 'ui-metric-value--mono' : ''}`}>
        {typeof value === 'number' ? <span lang={locale}>{number(value, { maximumSignificantDigits: 21 })}</span> : value}
      </div>
      {caption && <div className="ui-metric-tile__caption">{caption}</div>}
    </div>
  );
};
