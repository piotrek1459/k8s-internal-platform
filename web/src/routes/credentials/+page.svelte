<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { listCredentials, createCredential, deleteCredential, ApiError } from '$lib/api.js';
	import type { RegistryCredential } from '$lib/types.js';

	let credentials = $state<RegistryCredential[]>([]);
	let loading = $state(true);
	let error = $state('');
	let showForm = $state(false);

	// Create form
	let label = $state('');
	let registry = $state('');
	let username = $state('');
	let password = $state('');
	let creating = $state(false);
	let createError = $state('');

	onMount(load);

	async function load() {
		try {
			const res = await listCredentials();
			credentials = res.items;
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) goto('/login');
			else if (e instanceof ApiError && e.status === 404) {
				// Registry credentials endpoint not yet available — show empty state
				credentials = [];
			} else {
				error = e instanceof Error ? e.message : 'Failed to load credentials';
			}
		} finally {
			loading = false;
		}
	}

	async function handleCreate(e: Event) {
		e.preventDefault();
		createError = '';
		creating = true;
		try {
			await createCredential(label.trim(), registry.trim(), username.trim(), password);
			label = registry = username = password = '';
			showForm = false;
			await load();
		} catch (e) {
			createError = e instanceof ApiError ? e.message : 'Failed to add credential';
		} finally {
			creating = false;
		}
	}

	async function handleDelete(id: string) {
		if (!confirm('Remove this registry credential?')) return;
		try {
			await deleteCredential(id);
			await load();
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Failed to remove credential';
		}
	}
</script>

<div class="flex items-center justify-between mb-6">
	<h1 class="text-xl font-semibold text-gray-900">Registry credentials</h1>
	<button onclick={() => showForm = !showForm}
		class="bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium px-4 py-2 rounded transition-colors">
		Add credential
	</button>
</div>

{#if loading}
	<p class="text-gray-500 text-sm">Loading…</p>
{:else}
	{#if error}
		<div class="mb-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">{error}</div>
	{/if}

	{#if showForm}
		<div class="bg-white rounded border border-gray-200 p-4 mb-6">
			<h2 class="text-sm font-medium text-gray-700 mb-3">Add registry credential</h2>
			{#if createError}
				<div class="mb-3 p-2 bg-red-50 border border-red-200 rounded text-xs text-red-700">{createError}</div>
			{/if}
			<form onsubmit={handleCreate} class="space-y-3">
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label for="cred-label" class="block text-xs font-medium text-gray-600 mb-1">Label</label>
						<input id="cred-label" type="text" bind:value={label} placeholder="My registry" required
							class="w-full border border-gray-300 rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
					</div>
					<div>
						<label for="cred-registry" class="block text-xs font-medium text-gray-600 mb-1">Registry hostname</label>
						<input id="cred-registry" type="text" bind:value={registry} placeholder="ghcr.io" required
							class="w-full border border-gray-300 rounded px-2 py-1.5 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500" />
					</div>
					<div>
						<label for="cred-username" class="block text-xs font-medium text-gray-600 mb-1">Username</label>
						<input id="cred-username" type="text" bind:value={username} required
							class="w-full border border-gray-300 rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
					</div>
					<div>
						<label for="cred-password" class="block text-xs font-medium text-gray-600 mb-1">Password / token</label>
						<input id="cred-password" type="password" bind:value={password} required
							class="w-full border border-gray-300 rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
					</div>
				</div>
				<div class="flex gap-2">
					<button type="submit" disabled={creating}
						class="px-4 py-1.5 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white text-sm rounded transition-colors">
						{creating ? 'Adding…' : 'Add credential'}
					</button>
					<button type="button" onclick={() => showForm = false}
						class="px-4 py-1.5 border border-gray-300 text-sm rounded hover:bg-gray-50 transition-colors">Cancel</button>
				</div>
			</form>
		</div>
	{/if}

	{#if credentials.length === 0}
		<div class="text-center py-12 text-gray-400">
			<p class="text-sm">No registry credentials yet.</p>
			<p class="text-xs mt-1">Add one to deploy private images.</p>
		</div>
	{:else}
		<div class="bg-white rounded border border-gray-200 divide-y divide-gray-100">
			{#each credentials as cred}
				<div class="flex items-center justify-between px-4 py-3">
					<div>
						<p class="text-sm font-medium text-gray-900">{cred.label}</p>
						<p class="text-xs text-gray-400 mt-0.5">{cred.registry} · {cred.username}</p>
					</div>
					<button onclick={() => handleDelete(cred.id)}
						class="text-xs text-red-500 hover:underline">Remove</button>
				</div>
			{/each}
		</div>
	{/if}
{/if}
