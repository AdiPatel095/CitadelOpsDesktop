import { API_CONFIG } from '../config/Api';

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
