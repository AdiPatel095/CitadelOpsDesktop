import React, { HTMLAttributes } from 'react';

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  variant?: 'glass' | 'solid' | 'interactive';
}

export const Card = React.forwardRef<HTMLDivElement, CardProps>(
  ({ className = '', variant = 'glass', children, ...props }, ref) => {
    const baseStyles = 'rounded-global border transition-all duration-300 backdrop-blur-2xl';
    
    const variants = {
      glass: 'bg-bg-card/55 border-border-light shadow-[var(--shadow-glass-panel)]',
      solid: 'bg-bg-card/82 border-border-base shadow-[var(--glass-shadow-compact)]',
      interactive: 'bg-bg-card/55 border-border-light shadow-[var(--shadow-glass-panel)] hover:bg-bg-card-hover/80 hover:border-primary/30 cursor-pointer hover:-translate-y-0.5',
    };

    return (
      <div
        ref={ref}
        className={`${baseStyles} ${variants[variant]} ${className}`}
        {...props}
      >
        {children}
      </div>
    );
  }
);

Card.displayName = 'Card';

export const CardHeader: React.FC<HTMLAttributes<HTMLDivElement>> = ({ className = '', children, ...props }) => (
  <div className={`px-5 py-4 border-b border-border-base/80 bg-white/[0.02] flex items-center justify-between ${className}`} {...props}>
    {children}
  </div>
);

export const CardTitle: React.FC<HTMLAttributes<HTMLHeadingElement>> = ({ className = '', children, ...props }) => (
  <h3 className={`text-lg font-bold text-text-main ${className}`} {...props}>
    {children}
  </h3>
);

export const CardContent: React.FC<HTMLAttributes<HTMLDivElement>> = ({ className = '', children, ...props }) => (
  <div className={`p-5 ${className}`} {...props}>
    {children}
  </div>
);
