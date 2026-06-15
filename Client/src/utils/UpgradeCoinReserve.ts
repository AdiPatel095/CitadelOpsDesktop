/** True when coin balance is at or below the configured upgrade reserve. */
export function coinsUnderUpgradeReserve(coins: number, threshold: number): boolean {
  const reserve = Number.isFinite(threshold) && threshold >= 0 ? threshold : 0;
  return Number.isFinite(coins) && coins <= reserve;
}
