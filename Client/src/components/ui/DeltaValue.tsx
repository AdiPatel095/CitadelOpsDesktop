import { type ReactNode } from 'react';
import { deltaTone, withDeltaSign } from './delta.ts';
import './delta.css';

export function Delta({ value, children }: { value: number; children: ReactNode }) {
  const tone = deltaTone(value);
  return <bdi className={`ui-delta ui-delta--${tone}`} data-delta={tone}>{withDeltaSign('', value)}{children}</bdi>;
}
