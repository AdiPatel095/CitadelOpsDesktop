import React, { HTMLAttributes } from 'react';

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  variant?: 'glass' | 'solid' | 'interactive';
}

export const Card = React.forwardRef<HTMLDivElement, CardProps>(
  ({ className = '', variant = 'glass', children, ...props }, ref) => {
    const baseStyles = 'rounded-global border transition-colors duration-300';
    
    const variants = {
      glass: 'bg-bg-card/80 backdrop-blur-md border-border-base shadow-lg',
      solid: 'bg-bg-card border-border-base shadow-sm',
      interactive: 'bg-bg-card/80 backdrop-blur-md border-border-base shadow-lg hover:bg-bg-card-hover cursor-pointer',
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
  <div className={`px-5 py-4 border-b border-border-base flex items-center justify-between ${className}`} {...props}>
    {children}
  </div>
);

export const CardTitle: React.FC<HTMLAttributes<HTMLHeadingElement>> = ({ className = '', children, ...props }) => (
  <h3 className={`text-lg font-bold text-text-main tracking-tight ${className}`} {...props}>
    {children}
  </h3>
);

export const CardContent: React.FC<HTMLAttributes<HTMLDivElement>> = ({ className = '', children, ...props }) => (
  <div className={`p-5 ${className}`} {...props}>
    {children}
  </div>
);
