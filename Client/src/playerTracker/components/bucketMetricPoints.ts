import type { TrackerMetricPoint, RangeKey } from './PlayerTrackerView';

export function bucketMetricPoints(
  points: TrackerMetricPoint[],
  range: RangeKey,
  bucketSecondsOverride?: number,
): TrackerMetricPoint[] {
  const bucketSeconds = bucketSecondsOverride ?? (range === '24h'
    ? 60
    : range === '7d'
      ? 60 * 60
      : 24 * 60 * 60);
  const buckets = new Map<number, TrackerMetricPoint>();
  for (const point of points) {
    const bucket = Math.floor(point.timestampUnix / bucketSeconds);
    const existing = buckets.get(bucket);
    // Retention keeps the observation closest to the start of each UTC
    // bucket, so rendering follows the same rule even before a prune runs.
    if (!existing || point.timestampUnix < existing.timestampUnix) {
      buckets.set(bucket, point);
    }
  }
  return [...buckets.values()].sort((left, right) => left.timestampUnix - right.timestampUnix);
}
