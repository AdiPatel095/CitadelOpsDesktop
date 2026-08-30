import { API_CONFIG } from '../config/Api';

let configurationBasePath: string | null = null;

// Hosted command centers install an account-owned control-plane prefix (for
// example `/api/hosted/accounts/{uuid}`) before mounting. Runtime/game traffic
// remains on the live shard; only durable configuration uses this override.
export function setConfigurationBasePath(prefix: string | null): void {
	const normalized = prefix?.trim().replace(/\/$/, '') ?? '';
	configurationBasePath = normalized || null;
}

export function hasExternalConfiguration(): boolean {
	return configurationBasePath !== null;
}

// Capture this value when a save is queued. Hosted account navigation can
// replace the module-level prefix before an earlier async save begins; a
// bound URL keeps that write on the account where the user initiated it.
export function configurationBaseURL(): string {
	if (configurationBasePath !== null) return `${configurationBasePath}/config`;
	return `${runtimeBasePath()}/api/v2/config`;
}

export function runtimeBasePath(): string {
	const configured = API_CONFIG.BASE_URL.replace(/\/$/, '');
	if (configured) return configured;
	if (typeof window === 'undefined') return '';
	const match = window.location.pathname.match(/^\/accounts\/([a-z0-9][a-z0-9_-]{0,63})(?:\/|$)/);
	return match ? `/accounts/${match[1]}` : '';
}

export function runtimeURL(path: string): string {
	const normalized = path.startsWith('/') ? path : `/${path}`;
	return `${runtimeBasePath()}${normalized}`;
}

// Tenant runtime cookies live on the account shard. `include` is required when
// the persistent frontend and the cell use separate allowlisted origins, and
// is equivalent to the fetch default for the ordinary same-origin desktop.
export function runtimeFetch(path: string, init: RequestInit = {}): Promise<Response> {
	return fetch(runtimeURL(path), {
		...init,
		credentials: init.credentials ?? 'include',
	});
}

export function configurationFetch(
	path: string,
	init: RequestInit = {},
	baseURL = configurationBaseURL(),
): Promise<Response> {
	const suffix = path.startsWith('/') ? path : `/${path}`;
	return fetch(`${baseURL}${suffix === '/' ? '' : suffix}`, {
		...init,
		credentials: init.credentials ?? 'include',
	});
}
