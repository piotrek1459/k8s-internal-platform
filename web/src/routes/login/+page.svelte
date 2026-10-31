<script lang="ts">
	import { goto } from '$app/navigation';
	import { login, ApiError } from '$lib/api.js';
	import { session } from '$lib/auth.js';

	let password = $state('');
	let error = $state('');
	let submitting = $state(false);

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		submitting = true;
		try {
			await login(password);
			await session.check();
			goto('/apps');
		} catch (e) {
			error = e instanceof ApiError && e.status === 401
				? 'Incorrect password.'
				: 'Login failed. Please try again.';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="w-full max-w-sm">
	<h1 class="text-2xl font-semibold text-gray-900 mb-6">Sign in</h1>

	{#if error}
		<div class="mb-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">{error}</div>
	{/if}

	<form onsubmit={handleSubmit} class="space-y-4">
		<div>
			<label for="password" class="block text-sm font-medium text-gray-700 mb-1">Password</label>
			<input
				id="password"
				type="password"
				bind:value={password}
				required
				autocomplete="current-password"
				class="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
			/>
		</div>
		<button
			type="submit"
			disabled={submitting}
			class="w-full bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white font-medium py-2 px-4 rounded text-sm transition-colors"
		>
			{submitting ? 'Signing in…' : 'Sign in'}
		</button>
	</form>
</div>
