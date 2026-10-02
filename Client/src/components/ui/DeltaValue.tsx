import { type ReactNode } from 'react';
import { deltaTone, withDeltaSign } from './delta.ts';
import './delta.css';

export function DeltaValue({ value, children }: { value: number; children: ReactNode }) {
  const tone = deltaTone(value);
  const formattedWithSign = withDeltaSign(String(children), value);
  return <bdi className={`ui-delta ui-delta--${tone}`} data-delta={tone}>{formattedWithSign}</bdi>;
}
