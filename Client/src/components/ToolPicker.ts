import type { Dispatch, SetStateAction } from 'react';

export type ToolPickerSelectionMode = 'single' | 'multi';

export interface ToolPickerOptions {
  mode: ToolPickerSelectionMode;
  title?: string;
  preselected?: number[];
  allowedToolIds?: number[];
  /** Optional available stock counts shown on each tool card. */
  stockQuantities?: Record<number, number>;
}

export type ToolPickerResult = number | number[] | null;

export const toolPickerBridge: {
  resolve: ((value: ToolPickerResult) => void) | null;
  setState: Dispatch<SetStateAction<{ isOpen: boolean; options: ToolPickerOptions | null }>> | null;
} = { resolve: null, setState: null };

export function showToolPicker(options: ToolPickerOptions): Promise<ToolPickerResult> {
  return new Promise((resolve) => {
    toolPickerBridge.resolve = resolve;
    if (toolPickerBridge.setState) {
      toolPickerBridge.setState({ isOpen: true, options });
    }
  });
}
