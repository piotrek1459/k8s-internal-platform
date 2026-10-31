<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { getSetupStatus, setup, ApiError } from '$lib/api.js';

	let available = $state(false);
	let checking = $state(true);
	let token = $state('');
	let password = $state('');
	let confirm = $state('');
	let error = $state('');
	let submitting = $state(false);

	onMount(async () => {
		try {
			const s = await getSetupStatus();
			if (!s.setupAvailable) goto('/login');
			available = true;
		} catch {
			goto('/login');
		} finally {
			checking = false;
		}
	});

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		if (password !== confirm) { error = 'Passwords do not match.'; return; }
		if (password.length < 12) { error = 'Password must be at least 12 characters.'; return; }
		submitting = true;
		try {
			await setup(token.trim(), password);
			goto('/login');
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Setup failed. Please try again.';
		} finally {
			submitting = false;
		}
	}
</script>

{#if checking}
	<p class="text-gray-500">Checking…</p>
{:else if available}
	<div class="w-full max-w-md">
		<h1 class="text-2xl font-semibold text-gray-900 mb-2">Platform setup</h1>
		<p class="text-sm text-gray-500 mb-6">
			Enter the one-time bootstrap token from the broker logs and choose an administrator password.
		</p>

		{#if error}
			<div class="mb-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">{error}</div>
		{/if}

		<form onsubmit={handleSubmit} class="space-y-4">
			<div>
				<label for="token" class="block text-sm font-medium text-gray-700 mb-1">Bootstrap token</label>
				<input
					id="token"
					type="text"
					bind:value={token}
					required
					autocomplete="off"
					class="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono"
					placeholder="ohiM-8Qr5Lu7…"
				/>
			</div>
			<div>
				<label for="password" class="block text-sm font-medium text-gray-700 mb-1">Administrator password</label>
				<input
					id="password"
					type="password"
					bind:value={password}
					required
					autocomplete="new-password"
					class="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
				/>
			</div>
			<div>
				<label for="confirm" class="block text-sm font-medium text-gray-700 mb-1">Confirm password</label>
				<input
					id="confirm"
					type="password"
					bind:value={confirm}
					required
					autocomplete="new-password"
					class="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
				/>
			</div>
			<button
				type="submit"
				disabled={submitting}
				class="w-full bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white font-medium py-2 px-4 rounded text-sm transition-colors"
			>
				{submitting ? 'Creating account…' : 'Create account'}
			</button>
		</form>
	</div>
{/if}
