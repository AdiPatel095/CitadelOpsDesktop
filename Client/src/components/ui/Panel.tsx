import { type ElementType, type HTMLAttributes } from 'react';
import './Card.css';

export interface PanelProps extends HTMLAttributes<HTMLElement> {
  as?: ElementType;
}
export const Panel = ({ as: Component = 'div', className = '', children, ...props }: PanelProps) => (
  <Component className={`ui-panel ${className}`} {...props}>{children}</Component>
);
