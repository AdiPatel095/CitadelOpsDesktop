import type { SettingsBundleV1 } from '../api/Contracts';

const portableStorageKeys = new Set([
	'theme',
	'unitPicker_favorites',
	'unitPicker_frequentlyUsed',
	'equipmentAutoSellNonRelicEquipment',
	'equipmentAutoSellNonRelicEquipmentIntervalMinutes',
]);

const portableStoragePrefixes = [
	'citadel.equipment.optimizer.',
];

export function withPortableClientPreferences(bundle: SettingsBundleV1): SettingsBundleV1 {
	return {
		...bundle,
		clientPreferences: readPortableClientPreferences(),
	};
}

export function parseSettingsBundle(contents: string): SettingsBundleV1 {
	let value: unknown;
	try {
		value = JSON.parse(contents);
	} catch {
		throw new Error('The selected file is not valid JSON.');
	}
	if (!isRecord(value) || value.format !== 'citadelops-settings' || value.formatVersion !== 1) {
		throw new Error('This is not a supported CitadelOps settings export.');
	}
	if (typeof value.exportedAt !== 'string' || !isRecord(value.configuration)) {
		throw new Error('The settings export is missing its configuration metadata.');
	}
	const configuration = value.configuration;
	if (!Number.isSafeInteger(configuration.schemaVersion) || !isRecord(configuration.sections)) {
		throw new Error('The settings export has an invalid configuration payload.');
	}
	if (Object.keys(configuration.sections).length === 0) {
		throw new Error('The settings export contains no configuration sections.');
	}
	if (value.clientPreferences !== undefined) {
		if (!isRecord(value.clientPreferences) ||
			Object.values(value.clientPreferences).some((preference) => typeof preference !== 'string')) {
			throw new Error('The settings export has invalid client preferences.');
		}
		value.clientPreferences = Object.fromEntries(
			Object.entries(value.clientPreferences).filter(([key]) => isPortableStorageKey(key)),
		);
	}
	return value as unknown as SettingsBundleV1;
}

export function applyPortableClientPreferences(preferences: Record<string, string> | undefined): number {
	if (preferences === undefined) return 0;
	let existingKeys: string[] = [];
	const previous = new Map<string, string>();
	try {
		existingKeys = storageKeys().filter(isPortableStorageKey);
		for (const key of existingKeys) {
			const value = localStorage.getItem(key);
			if (value != null) previous.set(key, value);
		}
		for (const key of existingKeys) localStorage.removeItem(key);
		for (const [key, value] of Object.entries(preferences)) localStorage.setItem(key, value);
	} catch {
		try {
			for (const key of storageKeys().filter(isPortableStorageKey)) localStorage.removeItem(key);
			for (const [key, value] of previous) localStorage.setItem(key, value);
		} catch {
			// The server configuration is already imported; the reload will still use those settings.
		}
		throw new Error('App settings were imported, but local display preferences could not be applied.');
	}
	return Object.keys(preferences).length;
}

function readPortableClientPreferences(): Record<string, string> {
	const preferences: Record<string, string> = {};
	try {
		for (const key of storageKeys()) {
			if (!isPortableStorageKey(key)) continue;
			const value = localStorage.getItem(key);
			if (value != null) preferences[key] = value;
		}
	} catch {
		return {};
	}
	return preferences;
}

function storageKeys(): string[] {
	const keys: string[] = [];
	for (let index = 0; index < localStorage.length; index++) {
		const key = localStorage.key(index);
		if (key != null) keys.push(key);
	}
	return keys;
}

function isPortableStorageKey(key: string): boolean {
	return portableStorageKeys.has(key) || portableStoragePrefixes.some((prefix) => key.startsWith(prefix));
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value != null && !Array.isArray(value);
}
