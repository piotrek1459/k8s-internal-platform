<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { getApp, ApiError } from '$lib/api.js';
	import StatusBadge from '$components/StatusBadge.svelte';
	import type { Application } from '$lib/types.js';

	const id = $derived(page.params.id ?? '');
	let app = $state<Application | null>(null);
	let loading = $state(true);
	let error = $state('');
	let activeTab = $state<'overview' | 'events' | 'history'>('overview');

	onMount(async () => {
		try {
			app = await getApp(id);
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) goto('/login');
			else if (e instanceof ApiError && e.status === 404) error = 'Application not found.';
			else error = e instanceof Error ? e.message : 'Failed to load application';
		} finally {
			loading = false;
		}
	});
</script>

{#if loading}
	<p class="text-gray-500 text-sm">Loading…</p>
{:else if error}
	<div class="p-4 bg-red-50 border border-red-200 rounded text-sm text-red-700">{error}</div>
{:else if app}
	<div class="flex items-start justify-between mb-6">
		<div>
			<div class="flex items-center gap-3 mb-1">
				<h1 class="text-xl font-semibold text-gray-900">{app.name}</h1>
				<StatusBadge phase={app.status?.phase ?? 'unknown'} />
			</div>
			<p class="text-xs text-gray-400 font-mono">{app.id}</p>
		</div>
		<a href="/apps" class="text-sm text-gray-500 hover:text-gray-700">← Applications</a>
	</div>

	<!-- Tabs -->
	<div class="border-b border-gray-200 mb-6 flex gap-6">
		{#each [['overview', 'Overview'], ['events', 'Events'], ['history', 'History']] as [tab, label]}
			<button
				onclick={() => activeTab = tab as typeof activeTab}
				class="pb-2 text-sm border-b-2 transition-colors
					{activeTab === tab ? 'border-blue-600 text-blue-700 font-medium' : 'border-transparent text-gray-500 hover:text-gray-700'}"
			>{label}</button>
		{/each}
	</div>

	{#if activeTab === 'overview'}
		<dl class="grid grid-cols-2 gap-4 text-sm max-w-lg">
			<div>
				<dt class="text-gray-500 text-xs mb-0.5">Ready replicas</dt>
				<dd class="font-medium">{app.status?.readyReplicas ?? '—'} / {app.status?.desiredReplicas ?? '—'}</dd>
			</div>
			<div>
				<dt class="text-gray-500 text-xs mb-0.5">Workload ready</dt>
				<dd class="font-medium">{app.status?.workloadReady ? 'Yes' : 'No'}</dd>
			</div>
			<div>
				<dt class="text-gray-500 text-xs mb-0.5">Active operation</dt>
				<dd class="font-medium font-mono text-xs">{app.activeOperationId || '—'}</dd>
			</div>
			<div>
				<dt class="text-gray-500 text-xs mb-0.5">Observed at</dt>
				<dd class="font-medium text-xs">{app.status?.observedAt ? new Date(app.status.observedAt).toLocaleString() : '—'}
					{#if app.status?.stale}<span class="text-yellow-600 text-xs ml-1">(stale)</span>{/if}
				</dd>
			</div>
			<div class="col-span-2">
				<dt class="text-gray-500 text-xs mb-0.5">Last successful revision</dt>
				<dd class="font-medium font-mono text-xs">{app.lastSuccessfulRevision || '—'}</dd>
			</div>
		</dl>
	{:else if activeTab === 'events'}
		<p class="text-sm text-gray-500">
			{#if app.activeOperationId}
				<a href="/apps/{app.id}/operation?op={app.activeOperationId}" class="text-blue-600 hover:underline">
					View active operation → {app.activeOperationId}
				</a>
			{:else}
				No active operation.
			{/if}
		</p>
	{:else if activeTab === 'history'}
		<p class="text-sm text-gray-500">
			Desired revision: <span class="font-mono">{app.desiredRevision || '—'}</span>
		</p>
	{/if}
{/if}
