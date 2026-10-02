import type { ReactNode } from 'react';
import { AlertTriangle } from 'lucide-react';
import { Button } from './Button';
import { EmptyState, type EmptyStateProps } from './EmptyState';
import type { ViewStatus } from './viewStatus';
import './ViewState.css';
export interface ErrorStateProps { title: ReactNode; description?: ReactNode; onRetry?: () => void; retryLabel?: string; }
export interface LoadingStateProps { label: string; variant?: 'list' | 'table' | 'cards'; rows?: number; }
export function ErrorState({ title, description, onRetry, retryLabel, size = 'md' }: ErrorStateProps & { size?: EmptyStateProps['size'] }) {
  return <EmptyState role="status" className="ui-error-state" size={size} title={title} description={description} icon={<AlertTriangle />}
    action={onRetry && retryLabel ? <Button type="button" variant="secondary" onClick={onRetry}>{retryLabel}</Button> : undefined} />;
}
export function LoadingState({ label, variant = 'list', rows = 3, size = 'md' }: LoadingStateProps & { size?: EmptyStateProps['size'] }) {
  return <div role="status" aria-busy="true" className={`ui-loading-state ui-loading-state--${variant} ui-state--${size}`}>
    <span className="ui-state__sr-only">{label}</span>
    <div className="ui-loading-state__blocks" aria-hidden="true">{Array.from({ length: Math.max(1, rows) }, (_, index) => <span className="ui-loading-state__block" key={index} />)}</div>
  </div>;
}
export function ViewState({ status, size = 'md', error, loading, empty, children }: {
  status: ViewStatus; size?: EmptyStateProps['size']; error: ErrorStateProps; loading: LoadingStateProps; empty: EmptyStateProps; children?: ReactNode;
}) {
  return <div className="ui-view-state" data-state={status}>{status === 'content' ? children : status === 'error' ? <ErrorState {...error} size={size} /> : status === 'loading' ? <LoadingState {...loading} size={size} /> : <EmptyState {...empty} size={size} />}</div>;
}
