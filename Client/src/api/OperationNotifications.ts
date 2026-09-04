import type { IntentFailurePresentation, IntentReceipt, IntentStatus } from './Contracts';

export interface OperationFailureNotification {
	category: 'yellow' | 'red';
	message: string;
	lines?: string[];
}

export interface RoutedOperationFailureNotification extends OperationFailureNotification {
	id: string;
}

export class OperationFailureNotificationCoordinator {
	private readonly notified = new Set<string>();
	private readonly maximumEntries: number;

	constructor(maximumEntries = 512) {
		this.maximumEntries = maximumEntries;
	}

	next(receipt: IntentReceipt, preferredID?: string): RoutedOperationFailureNotification | null {
		if (this.notified.has(receipt.id)) return null;
		const notification = operationFailureNotification(receipt);
		if (!notification) return null;
		this.notified.add(receipt.id);
		while (this.notified.size > this.maximumEntries) {
			const oldest = this.notified.values().next().value;
			if (typeof oldest !== 'string') break;
			this.notified.delete(oldest);
		}
		return { ...notification, id: operationNotificationID(receipt.id, preferredID) };
	}
}

export function operationNotificationID(operationID: string, preferredID?: string): string {
	return cleanText(preferredID) || `operation-${cleanText(operationID)}`;
}

interface LegacyGameFailure {
	code: number;
	opcode: string;
	explanation: string;
	knowledge: 'official' | 'observed' | 'unknown';
}

interface GameGuidance {
	recovery: string;
	expectedState: boolean;
}

const legacyGameGuidance: Record<string, GameGuidance> = {
	'*:53': {
		recovery: 'Refresh the affected feature and retry after its current game context is restored.',
		expectedState: true,
	},
	'*:147': {
		recovery: 'Refresh the feature before retrying; the requested process may already be complete.',
		expectedState: true,
	},
	'*:175': {
		recovery: 'Choose a location in a kingdom this account can currently access, then refresh the feature.',
		expectedState: true,
	},
	'adi:95': {
		recovery: 'Wait for the target cooldown to end, or refresh the world map before choosing another target.',
		expectedState: true,
	},
	'bup:87': {
		recovery: "Refresh recruitable troops and the castle's production buildings before choosing another troop.",
		expectedState: true,
	},
	'cds:101': {
		recovery: 'Refresh the troop selection before trying again.',
		expectedState: true,
	},
	'cra:256': {
		recovery: 'Wait for a commander to return. Automated combat pauses after this response to avoid repeated rejected launches.',
		expectedState: true,
	},
	'cra:91': {
		recovery: 'Remove or replace the incompatible tools in the selected attack preset, then retry.',
		expectedState: false,
	},
	'jaa:337': {
		recovery: 'Unlock or enter this kingdom in the game, then refresh the feature.',
		expectedState: true,
	},
	'msk:182': {
		recovery: 'Refresh kingdom transport state before selecting another time skip.',
		expectedState: true,
	},
	'rae:327': {
		recovery: 'Refresh the event and choose one of the currencies it currently offers.',
		expectedState: true,
	},
	'sbp:55': {
		recovery: 'Wait for enough shop currency or lower the purchase amount, then refresh the shop.',
		expectedState: true,
	},
	'sbp:159': {
		recovery: 'Refresh the shop before choosing an available offer again.',
		expectedState: true,
	},
	'sbp:203': {
		recovery: 'Refresh the shop before choosing an available offer again.',
		expectedState: true,
	},
};

const technicalFailurePattern = /\b(?:opcode|payload|resolver|dependency|intent|operation|revision|generation|cursor|json field|response code|observer)\b|\b(?:AID|CID|KID|LID|OID|PID|SID|TID|WID|WOD|AMT|CRA|GAA|SBP)\s*[:=]/i;
const unresolvedGameTextPlaceholder = /\{[0-9]+\}/;

export function isOperationFailureStatus(status: IntentStatus): boolean {
	return status === 'failed' || status === 'partially_succeeded' || status === 'indeterminate';
}

export function operationFailureNotification(receipt: IntentReceipt): OperationFailureNotification | null {
	if (!isOperationFailureStatus(receipt.status)) return null;
	const structured = validFailurePresentation(receipt.failure) ? receipt.failure : undefined;
	if (structured) {
		if (!structured.toast && receipt.status === 'failed') return null;
		return notificationFromStructuredFailure(structured);
	}
	return legacyFailureNotification(receipt);
}

export function operationFailureText(receipt: IntentReceipt): string {
	const structured = validFailurePresentation(receipt.failure) ? receipt.failure : undefined;
	const notification = structured
		? notificationFromStructuredFailure(structured)
		: legacyFailureNotification(receipt, true);
	if (!notification) return cleanText(receipt.error) || 'This action could not be completed.';
	return [notification.message, ...(notification.lines ?? [])].join(' ');
}

function notificationFromStructuredFailure(failure: IntentFailurePresentation): OperationFailureNotification {
	const explanation = cleanText(failure.explanation) || 'The action did not complete.';
	const lines = [knowledgeExplanation(explanation, failure.knowledge)];
	const recovery = cleanText(failure.recovery);
	if (recovery && recovery.toLowerCase() !== explanation.toLowerCase()) lines.push(recovery);
	if (Number.isSafeInteger(failure.gameCode)) lines.push(`Game error ${failure.gameCode}.`);
	return {
		category: failure.severity === 'warning' ? 'yellow' : 'red',
		message: cleanText(failure.message) || 'This action could not be completed.',
		lines: uniqueLines(lines),
	};
}

function legacyFailureNotification(
	receipt: IntentReceipt,
	includeLaneStatusOnly = false,
): OperationFailureNotification | null {
	const raw = cleanText(receipt.error);
	const lower = raw.toLowerCase();
	const gameFailure = parseLegacyGameFailure(raw);
	if (gameFailure) {
		if (legacyResponseCodeSafetyFailure(lower)) {
			return {
				category: 'red',
				message: failureHeadline(receipt),
				lines: [
					'The game rejected this action, and an earlier game confirmation could not be applied safely.',
					'Refresh the feature and verify the current game state before retrying.',
					`Game error ${gameFailure.code}.`,
				],
			};
		}
		const guidance = gameGuidance(gameFailure.opcode, gameFailure.code);
		if (!includeLaneStatusOnly && receipt.status === 'failed' && automationActor(receipt.actor) && guidance?.expectedState) return null;
		const explanation = gameFailure.knowledge === 'unknown'
			? 'The game declined this action but does not provide a known explanation.'
			: unresolvedGameTextPlaceholder.test(gameFailure.explanation)
				? 'The game declined this action, but its published explanation was incomplete.'
				: gameFailure.explanation;
		const lines = [knowledgeExplanation(explanation, gameFailure.knowledge)];
		if (guidance?.recovery) lines.push(guidance.recovery);
		else if (gameFailure.knowledge === 'unknown') {
			lines.push(`Refresh the feature once before retrying. If it repeats, include game error ${gameFailure.code} when reporting it.`);
		}
		lines.push(`Game error ${gameFailure.code}.`);
		return {
			category: guidance?.expectedState ? 'yellow' : 'red',
			message: failureHeadline(receipt),
			lines: uniqueLines(lines),
		};
	}

	if (commanderAvailabilityFailure(lower)) {
		if (!includeLaneStatusOnly && receipt.status === 'failed' && automationActor(receipt.actor)) return null;
		const assigned = lower.includes('no commanders are assigned');
		const requirements = lower.includes('supports the required') || lower.includes('current roster');
		return {
			category: 'yellow',
			message: failureHeadline(receipt),
			lines: [
				assigned
					? 'No commander is assigned to this feature.'
					: requirements
						? "No assigned commander currently meets this feature's requirements."
						: 'No eligible commander is available right now.',
				assigned
					? 'Assign at least one eligible commander in the feature settings.'
					: requirements
						? 'Assign a commander that meets the feature requirements.'
						: 'Wait for a commander to return; the feature lane will reevaluate automatically.',
			],
		};
	}

	if (troopAvailabilityFailure(lower)) {
		if (!includeLaneStatusOnly && receipt.status === 'failed' && automationActor(receipt.actor)) return null;
		return {
			category: 'yellow',
			message: failureHeadline(receipt),
			lines: [
				'There are not enough eligible troops available for this action.',
				'The feature lane will reevaluate after troop availability changes.',
			],
		};
	}

	const known = legacyNonGameExplanation(lower);
	const explanation = (known?.explanation
		?? (technicalFailurePattern.test(raw) ? 'An internal app error prevented the action.' : raw))
		|| 'The action did not complete.';
	const lines = [explanation];
	if (known?.recovery) lines.push(known.recovery);
	return {
		category: known?.warning ? 'yellow' : 'red',
		message: failureHeadline(receipt),
		lines: uniqueLines(lines),
	};
}

function legacyNonGameExplanation(lower: string): { explanation: string; recovery?: string; warning?: boolean } | undefined {
	if (lower.includes('timed out waiting for') || lower.includes('context deadline exceeded')) return {
		explanation: 'The game did not confirm the action in time.',
		recovery: 'Check the game or feature status before retrying so a completed action is not repeated.',
		warning: true,
	};
	if (lower.includes('game session changed while waiting for') || lower.includes('game session changed while committing') || lower.includes('game websocket connection changed')) return {
		explanation: 'The game connection changed before the action could be confirmed.',
		recovery: 'Wait for the current game state to finish refreshing before trying again.',
		warning: true,
	};
	if (lower.includes('game websocket')) return {
		explanation: 'The game connection was unavailable.',
		recovery: 'Reconnect to the game before trying again.',
	};
	if (lower.includes('response did not include a result code')) return {
		explanation: 'The game returned a confirmation the app could not validate.',
		recovery: 'Refresh the feature before retrying. If it repeats, report the failed action.',
	};
	if (lower.includes('intent plan became stale')) return {
		explanation: 'The game state changed before the action finished.',
		recovery: 'Review the refreshed feature status before trying again.',
		warning: true,
	};
	if (lower.includes('outbound effect outcome is indeterminate')) return {
		explanation: 'The game did not confirm whether the action completed.',
		recovery: 'Check the game before retrying so a completed action is not duplicated.',
		warning: true,
	};
	if (lower.includes('persist ')) return {
		explanation: 'The app could not save the action state safely.',
		recovery: 'Do not repeat the action until storage is available and the feature status is current.',
	};
	if (lower.includes('response observer is unavailable')
		|| lower.includes('committed wire response observer is unavailable')
		|| (lower.includes('action "') && lower.includes('is not registered'))) return {
		explanation: 'An internal app error prevented the action.',
		recovery: 'If this repeats, report the action and time it occurred.',
	};
	return undefined;
}

function legacyResponseCodeSafetyFailure(lower: string): boolean {
	return lower.includes('outbound effect outcome is indeterminate')
		|| lower.includes('commit earlier acknowledged response')
		|| lower.includes('commit acknowledged response')
		|| lower.includes('response state reduction failed');
}

function parseLegacyGameFailure(error: string): LegacyGameFailure | undefined {
	const match = /response code\s+(-?\d+)(?:\s+for\s+([a-z0-9_-]+))?\s+was not successful:\s*([\s\S]+)$/i.exec(error);
	if (!match) return undefined;
	const code = Number(match[1]);
	if (!Number.isSafeInteger(code)) return undefined;
	let explanation = cleanText(match[3]);
	let knowledge: LegacyGameFailure['knowledge'] = 'unknown';
	const source = /\s+\((official game text|inferred from captures|undocumented)\)\s*$/i.exec(explanation);
	if (source) {
		explanation = cleanText(explanation.slice(0, source.index));
		if (source[1].toLowerCase() === 'official game text') knowledge = 'official';
		else if (source[1].toLowerCase() === 'inferred from captures') knowledge = 'observed';
	}
	return { code, opcode: cleanText(match[2]).toLowerCase(), explanation, knowledge };
}

function gameGuidance(opcode: string, code: number): GameGuidance | undefined {
	return legacyGameGuidance[`${opcode}:${code}`] ?? legacyGameGuidance[`*:${code}`];
}

function knowledgeExplanation(explanation: string, knowledge: IntentFailurePresentation['knowledge']): string {
	if (knowledge === 'official') return `The game says: ${explanation}`;
	if (knowledge === 'observed') return `Based on observed game behavior: ${explanation}`;
	return explanation;
}

function failureHeadline(receipt: IntentReceipt): string {
	let action = cleanText(receipt.plan?.summary).replace(/[.!?]+$/, '');
	if (action.length > 140) action = `${action.slice(0, 139).trim()}…`;
	if (!action) {
		if (receipt.status === 'partially_succeeded') return 'This action completed only in part.';
		if (receipt.status === 'indeterminate') return 'We could not confirm whether this action completed.';
		return 'This action could not be completed.';
	}
	if (receipt.status === 'partially_succeeded') return `“${action}” completed only in part.`;
	if (receipt.status === 'indeterminate') return `We could not confirm whether “${action}” completed.`;
	return `Could not complete “${action}”.`;
}

function troopAvailabilityFailure(lower: string): boolean {
	if (lower.includes('not enough troops') || lower.includes('insufficient troops')) return true;
	return lower.includes(' of item ')
		&& (lower.includes(' commander(s) require ') || lower.includes(' attack formation requires '));
}

function commanderAvailabilityFailure(lower: string): boolean {
	return lower.includes('no commander')
		|| lower.includes('no commanders')
		|| lower.includes('commander availability changed')
		|| (lower.includes('no available') && lower.includes('commander'))
		|| (lower.includes('commander') && (lower.includes(' is no longer available') || lower.includes(' is not available')))
		|| (lower.includes('no assigned') && lower.includes('commander'));
}

function automationActor(actor: string): boolean {
	return cleanText(actor).toLowerCase().startsWith('automation:');
}

function validFailurePresentation(value: IntentFailurePresentation | undefined): value is IntentFailurePresentation {
	return value != null
		&& (value.severity === 'warning' || value.severity === 'error')
		&& typeof value.message === 'string'
		&& typeof value.explanation === 'string'
		&& typeof value.toast === 'boolean';
}

function uniqueLines(lines: string[]): string[] {
	const seen = new Set<string>();
	return lines.map(cleanText).filter((line) => {
		if (!line || seen.has(line.toLowerCase())) return false;
		seen.add(line.toLowerCase());
		return true;
	});
}

function cleanText(value: unknown): string {
	return typeof value === 'string' ? value.trim().replace(/\s+/g, ' ') : '';
}
