import { writable } from 'svelte/store';
import { goto } from '$app/navigation';
import { getMe, logout as apiLogout, ApiError } from './api.js';

export interface SessionState {
	loading: boolean;
	administratorId: string | null;
	sessionExpiresAt: string | null;
}

function createSession() {
	const { subscribe, set } = writable<SessionState>({
		loading: true,
		administratorId: null,
		sessionExpiresAt: null
	});

	return {
		subscribe,
		async check() {
			try {
				const me = await getMe();
				set({ loading: false, administratorId: me.administratorId, sessionExpiresAt: me.sessionExpiresAt });
				return true;
			} catch (e) {
				set({ loading: false, administratorId: null, sessionExpiresAt: null });
				if (e instanceof ApiError && e.status === 401) return false;
				throw e;
			}
		},
		setUser(administratorId: string, sessionExpiresAt: string) {
			set({ loading: false, administratorId, sessionExpiresAt });
		},
		async logout() {
			try { await apiLogout(); } catch { /* ignore */ }
			set({ loading: false, administratorId: null, sessionExpiresAt: null });
			goto('/login');
		}
	};
}

export const session = createSession();

// Call from a page's +page.ts load function or layout to guard authenticated routes.
export async function requireAuth(): Promise<void> {
	const authed = await session.check();
	if (!authed) {
		goto('/login');
	}
}
