export type DeltaTone = 'gain' | 'loss' | 'zero';

export function deltaTone(value: number): DeltaTone {
  return value > 0 ? 'gain' : value < 0 ? 'loss' : 'zero';
}

export function withDeltaSign(formattedAbsolute: string, value: number): string {
  const tone = deltaTone(value);
  return `${tone === 'gain' ? '+' : tone === 'loss' ? '−' : ''}${formattedAbsolute}`;
}
