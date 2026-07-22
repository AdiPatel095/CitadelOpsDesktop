import React from 'react';
import {
  ToggleGroup,
  type ToggleGroupOption,
  type ToggleGroupProps,
} from './ToggleGroup';

export type PillSelectorOption = string | ToggleGroupOption;

export interface PillSelectorProps extends Omit<ToggleGroupProps, 'options'> {
  options: readonly PillSelectorOption[];
}

// PillSelector is the shared segmented-pill control. Its required context size keeps header and
// body instances on the same two geometries across the app.
export const PillSelector: React.FC<PillSelectorProps> = ({ options, className = '', ...props }) => {
  const normalizedOptions = options.map((option) => (
    typeof option === 'string' ? { value: option, label: option } : option
  ));

  return (
    <ToggleGroup
      {...props}
      options={normalizedOptions}
      className={`liquid-pill-selector ${className}`}
    />
  );
};
