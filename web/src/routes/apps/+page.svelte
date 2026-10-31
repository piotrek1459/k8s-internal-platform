<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { listApps, ApiError } from '$lib/api.js';
	import StatusBadge from '$components/StatusBadge.svelte';
	import type { ApplicationSummary } from '$lib/types.js';

	let items = $state<ApplicationSummary[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			const res = await listApps();
			items = res.items;
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) goto('/login');
			else error = e instanceof Error ? e.message : 'Failed to load applications';
		} finally {
			loading = false;
		}
	});
</script>

<div class="flex items-center justify-between mb-6">
	<h1 class="text-xl font-semibold text-gray-900">Applications</h1>
	<a
		href="/apps/new"
		class="bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium px-4 py-2 rounded transition-colors"
	>
		New application
	</a>
</div>

{#if loading}
	<p class="text-gray-500 text-sm">Loading…</p>
{:else if error}
	<p class="text-red-600 text-sm">{error}</p>
{:else if items.length === 0}
	<div class="text-center py-16 text-gray-400">
		<p class="text-lg mb-2">No applications yet</p>
		<a href="/apps/new" class="text-blue-600 hover:underline text-sm">Deploy your first application</a>
	</div>
{:else}
	<div class="bg-white rounded border border-gray-200 divide-y divide-gray-100">
		{#each items as app}
			<a
				href="/apps/{app.id}"
				class="flex items-center justify-between px-4 py-3 hover:bg-gray-50 transition-colors"
			>
				<div>
					<p class="text-sm font-medium text-gray-900">{app.name}</p>
					<p class="text-xs text-gray-400 mt-0.5">v{app.version} · {app.id}</p>
				</div>
				<StatusBadge phase={app.phase} />
			</a>
		{/each}
	</div>
{/if}
