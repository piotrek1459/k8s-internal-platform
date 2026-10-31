import type {
	AcceptResult,
	Application,
	ApplicationList,
	CreateApplicationRequest,
	ImageInspectResult,
	Me,
	Operation,
	RegistryCredential,
	Secret,
	SetupStatus,
	SystemInfo
} from './types.js';

export class ApiError extends Error {
	code: string;
	status: number;
	constructor(status: number, code: string, message: string) {
		super(message);
		this.status = status;
		this.code = code;
	}
}

const BASE_HEADERS: Record<string, string> = {
	'Content-Type': 'application/json',
	'X-Requested-With': 'XMLHttpRequest'
};

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
	const res = await fetch(path, {
		method,
		credentials: 'include',
		headers: BASE_HEADERS,
		...(body !== undefined ? { body: JSON.stringify(body) } : {})
	});
	if (res.ok) {
		if (res.status === 204) return undefined as T;
		return res.json() as Promise<T>;
	}
	let code = 'UNKNOWN';
	let message = res.statusText;
	try {
		const err = await res.json();
		code = err.code ?? code;
		message = err.message ?? message;
	} catch {
		// non-JSON error body
	}
	throw new ApiError(res.status, code, message);
}

// --- Setup & Auth ---

export function getSetupStatus(): Promise<SetupStatus> {
	return request('GET', '/api/v1/setup/status');
}

export function setup(token: string, password: string): Promise<void> {
	return requestWithHeaders('POST', '/api/v1/setup', { password }, { Authorization: `Bearer ${token}` });
}

export function login(password: string): Promise<void> {
	return request('POST', '/api/v1/auth/session', { password });
}

export function logout(): Promise<void> {
	return request('DELETE', '/api/v1/auth/session');
}

export function getMe(): Promise<Me> {
	return request('GET', '/api/v1/auth/me');
}

// --- System ---

export function getSystem(): Promise<SystemInfo> {
	return request('GET', '/api/v1/system');
}

// --- Applications ---

export function listApps(limit = 50, offset = 0): Promise<ApplicationList> {
	return request('GET', `/api/v1/apps?limit=${limit}&offset=${offset}`);
}

export function getApp(id: string): Promise<Application> {
	return request('GET', `/api/v1/apps/${id}`);
}

export function createApp(
	idempotencyKey: string,
	body: CreateApplicationRequest
): Promise<AcceptResult> {
	return requestWithHeaders('POST', '/api/v1/apps', body, { 'Idempotency-Key': idempotencyKey });
}

// --- Operations ---

export function getOperation(id: string): Promise<Operation> {
	return request('GET', `/api/v1/operations/${id}`);
}

// --- Images ---

export function inspectImage(
	image: string,
	registryCredentialId?: string
): Promise<ImageInspectResult> {
	return request('POST', '/api/v1/images/inspect', { image, registryCredentialId });
}

// --- Secrets ---

export function listSecrets(): Promise<{ items: Secret[] }> {
	return request('GET', '/api/v1/secrets');
}

export function createSecret(name: string, value: string): Promise<Secret> {
	return request('POST', '/api/v1/secrets', { name, data: { value } });
}

export function addSecretVersion(id: string, value: string): Promise<void> {
	return request('POST', `/api/v1/secrets/${id}/versions`, { data: { value } });
}

export function deleteSecret(id: string): Promise<void> {
	return request('DELETE', `/api/v1/secrets/${id}`);
}

// --- Registry Credentials ---

export function listCredentials(): Promise<{ items: RegistryCredential[] }> {
	return request('GET', '/api/v1/registry-credentials');
}

export function createCredential(
	label: string,
	registry: string,
	username: string,
	password: string
): Promise<RegistryCredential> {
	return request('POST', '/api/v1/registry-credentials', { label, registry, username, password });
}

export function deleteCredential(id: string): Promise<void> {
	return request('DELETE', `/api/v1/registry-credentials/${id}`);
}

// Internal helper that merges extra headers.
function requestWithHeaders<T>(
	method: string,
	path: string,
	body?: unknown,
	extra?: Record<string, string>
): Promise<T> {
	const headers = { ...BASE_HEADERS, ...extra };
	return fetch(path, {
		method,
		credentials: 'include',
		headers,
		...(body !== undefined ? { body: JSON.stringify(body) } : {})
	}).then(async (res) => {
		if (res.ok) {
			if (res.status === 204) return undefined as T;
			return res.json() as Promise<T>;
		}
		let code = 'UNKNOWN';
		let message = res.statusText;
		try {
			const err = await res.json();
			code = err.code ?? code;
			message = err.message ?? message;
		} catch {
			// non-JSON error body
		}
		throw new ApiError(res.status, code, message);
	});
}
