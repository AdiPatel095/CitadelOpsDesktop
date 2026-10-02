import { type ReactNode } from 'react';
import { Info, CircleCheck, CircleAlert, TriangleAlert, X } from 'lucide-react';
import { Button } from './Button';
import { useLocale } from '../../i18n/LocaleContext';
import './banner.css';

export interface BannerProps {
  tone: 'info' | 'success' | 'warning' | 'danger' | 'neutral';
  title?: ReactNode;
  children: ReactNode;
  action?: ReactNode;
  onDismiss?: () => void;
  dismissLabel?: string;
  role?: 'status' | 'alert';
}
const glyphs = { info: Info, success: CircleCheck, warning: CircleAlert, danger: TriangleAlert, neutral: Info };

export function Banner({ tone, title, children, action, onDismiss, dismissLabel, role }: BannerProps) {
  const Glyph = glyphs[tone];
  const { t } = useLocale();
  return (
    <div className="ui-banner" data-banner={tone} role={role}>
      <Glyph className="ui-banner__glyph" size={20} aria-hidden="true" />
      <div className="ui-banner__content">
        {title && <div className="ui-banner__title">{title}</div>}
        <div>{children}</div>
      </div>
      {action && <div className="ui-banner__action">{action}</div>}
      {onDismiss && <Button variant="ghost" size="icon" aria-label={dismissLabel ?? t('common.close')} onClick={onDismiss}><X size={16} aria-hidden="true" /></Button>}
    </div>
  );
}
