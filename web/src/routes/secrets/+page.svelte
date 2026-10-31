<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { listSecrets, createSecret, addSecretVersion, deleteSecret, ApiError } from '$lib/api.js';
	import type { Secret } from '$lib/types.js';

	let secrets = $state<Secret[]>([]);
	let loading = $state(true);
	let error = $state('');

	// Create form
	let newName = $state('');
	let newValue = $state('');
	let creating = $state(false);
	let createError = $state('');

	// Add version form
	let addingVersionFor = $state<string | null>(null);
	let newVersion = $state('');
	let addingVersion = $state(false);

	onMount(load);

	async function load() {
		try {
			const res = await listSecrets();
			secrets = res.items;
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) goto('/login');
			else error = e instanceof Error ? e.message : 'Failed to load secrets';
		} finally {
			loading = false;
		}
	}

	async function handleCreate(e: Event) {
		e.preventDefault();
		createError = '';
		creating = true;
		try {
			await createSecret(newName.trim(), newValue);
			newName = '';
			newValue = '';
			await load();
		} catch (e) {
			createError = e instanceof ApiError ? e.message : 'Failed to create secret';
		} finally {
			creating = false;
		}
	}

	async function handleAddVersion(id: string) {
		addingVersion = true;
		try {
			await addSecretVersion(id, newVersion);
			newVersion = '';
			addingVersionFor = null;
			await load();
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Failed to add version';
		} finally {
			addingVersion = false;
		}
	}

	async function handleDelete(id: string) {
		if (!confirm('Delete this secret? This cannot be undone.')) return;
		try {
			await deleteSecret(id);
			await load();
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Failed to delete secret';
		}
	}
</script>

<div class="flex items-center justify-between mb-6">
	<h1 class="text-xl font-semibold text-gray-900">Secrets</h1>
</div>

{#if loading}
	<p class="text-gray-500 text-sm">Loading…</p>
{:else}
	{#if error}
		<div class="mb-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">{error}</div>
	{/if}

	<!-- Create new secret -->
	<div class="bg-white rounded border border-gray-200 p-4 mb-6">
		<h2 class="text-sm font-medium text-gray-700 mb-3">New secret</h2>
		{#if createError}
			<div class="mb-3 p-2 bg-red-50 border border-red-200 rounded text-xs text-red-700">{createError}</div>
		{/if}
		<form onsubmit={handleCreate} class="flex gap-2">
			<input type="text" bind:value={newName} placeholder="Name" required
				class="w-36 border border-gray-300 rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
			<input type="password" bind:value={newValue} placeholder="Value (write-only)" required
				class="flex-1 border border-gray-300 rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
			<button type="submit" disabled={creating}
				class="px-3 py-1.5 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white text-sm rounded transition-colors">
				{creating ? 'Creating…' : 'Create'}
			</button>
		</form>
	</div>

	<!-- Secrets list -->
	{#if secrets.length === 0}
		<p class="text-gray-400 text-sm">No secrets yet.</p>
	{:else}
		<div class="bg-white rounded border border-gray-200 divide-y divide-gray-100">
			{#each secrets as secret}
				<div class="px-4 py-3">
					<div class="flex items-center justify-between">
						<div>
							<p class="text-sm font-medium text-gray-900">{secret.name}</p>
							<p class="text-xs text-gray-400 mt-0.5 font-mono">{secret.id}</p>
							{#if secret.versions?.length}
								<p class="text-xs text-gray-500 mt-0.5">Versions: {secret.versions.join(', ')}</p>
							{/if}
						</div>
						<div class="flex gap-2">
							<button onclick={() => addingVersionFor = addingVersionFor === secret.id ? null : secret.id}
								class="text-xs text-blue-600 hover:underline">Add version</button>
							<button onclick={() => handleDelete(secret.id)}
								class="text-xs text-red-500 hover:underline">Delete</button>
						</div>
					</div>
					{#if addingVersionFor === secret.id}
						<div class="mt-3 flex gap-2">
							<input type="password" bind:value={newVersion} placeholder="New value (write-only)"
								class="flex-1 border border-gray-300 rounded px-2 py-1.5 text-xs focus:outline-none focus:ring-2 focus:ring-blue-500" />
							<button onclick={() => handleAddVersion(secret.id)} disabled={addingVersion}
								class="px-3 py-1.5 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white text-xs rounded transition-colors">
								{addingVersion ? 'Adding…' : 'Add'}
							</button>
							<button onclick={() => addingVersionFor = null}
								class="px-2 py-1.5 border border-gray-300 text-xs rounded hover:bg-gray-50">Cancel</button>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
{/if}
