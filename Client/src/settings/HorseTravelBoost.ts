export type HorseTravelBoostID = -1 | 1007 | 1008 | 1009;

export const HORSE_TRAVEL_BOOST_OPTIONS: ReadonlyArray<{ value: string; label: string }> = [
  { value: '-1', label: 'Travel feather · HBW -1' },
  { value: '1007', label: 'Horse / ship tier · coins' },
  { value: '1008', label: 'Warhorse / fast ship tier · rubies' },
  { value: '1009', label: 'Courser / fastest ship tier · rubies' },
];

export function parseHorseTravelBoostID(value: unknown): HorseTravelBoostID {
  const parsed = Number(value);
  return parsed === 1007 || parsed === 1008 || parsed === 1009 ? parsed : -1;
}
