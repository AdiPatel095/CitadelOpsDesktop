import type { IntentReceipt } from './Contracts';

export function operationFailureReceiptFromHTTP(status: number, payload: unknown): IntentReceipt | undefined {
	if (status !== 422 || !isRecord(payload) || typeof payload.id !== 'string' ||
		typeof payload.intent !== 'string' || typeof payload.actor !== 'string') return undefined;
	if (payload.status !== 'failed' && payload.status !== 'partially_succeeded' && payload.status !== 'indeterminate') {
		return undefined;
	}
	return payload as unknown as IntentReceipt;
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}
