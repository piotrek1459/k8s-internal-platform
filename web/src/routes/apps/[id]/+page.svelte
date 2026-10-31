<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		getApp, listRevisions,
		restartApp, reapplyApp, rollbackApp, deleteApp,
		ApiError
	} from '$lib/api.js';
	import StatusBadge from '$components/StatusBadge.svelte';
	import type { Application, Revision } from '$lib/types.js';

	const id = $derived(page.params.id ?? '');
	let app = $state<Application | null>(null);
	let revisions = $state<Revision[]>([]);
	let loading = $state(true);
	let error = $state('');
	let activeTab = $state<'overview' | 'events' | 'history'>('overview');
	let actionError = $state('');
	let actionPending = $state(false);

	function newKey() {
		return crypto.randomUUID();
	}

	async function loadRevisions() {
		if (!app) return;
		try {
			const res = await listRevisions(id);
			revisions = res.items;
		} catch {
			// non-fatal
		}
	}

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

	async function handleRestart() {
		if (!app || actionPending) return;
		actionPending = true;
		actionError = '';
		try {
			const res = await restartApp(id, newKey(), app.version);
			goto(`/apps/${id}/operation?op=${res.operationId}`);
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Restart failed';
		} finally {
			actionPending = false;
		}
	}

	async function handleReapply() {
		if (!app || actionPending) return;
		actionPending = true;
		actionError = '';
		try {
			const res = await reapplyApp(id, newKey(), app.version);
			goto(`/apps/${id}/operation?op=${res.operationId}`);
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Reapply failed';
		} finally {
			actionPending = false;
		}
	}

	async function handleRollback(revisionId: string) {
		if (!app || actionPending) return;
		actionPending = true;
		actionError = '';
		try {
			const res = await rollbackApp(id, newKey(), revisionId, app.version);
			goto(`/apps/${id}/operation?op=${res.operationId}`);
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Rollback failed';
		} finally {
			actionPending = false;
		}
	}

	async function handleDelete() {
		if (!app || actionPending) return;
		if (!confirm(`Delete application "${app.name}"? This cannot be undone.`)) return;
		actionPending = true;
		actionError = '';
		try {
			const res = await deleteApp(id, newKey(), app.version);
			goto(`/apps/${id}/operation?op=${res.operationId}`);
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Delete failed';
		} finally {
			actionPending = false;
		}
	}
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

	<!-- Action bar -->
	<div class="flex flex-wrap gap-2 mb-6">
		<button
			onclick={handleRestart}
			disabled={actionPending || !!app.activeOperationId}
			class="px-3 py-1.5 text-sm bg-gray-100 hover:bg-gray-200 text-gray-700 rounded disabled:opacity-40 disabled:cursor-not-allowed"
		>Restart</button>
		<button
			onclick={handleReapply}
			disabled={actionPending || !!app.activeOperationId}
			class="px-3 py-1.5 text-sm bg-gray-100 hover:bg-gray-200 text-gray-700 rounded disabled:opacity-40 disabled:cursor-not-allowed"
		>Reapply</button>
		<a
			href="/apps/{id}/update"
			class="px-3 py-1.5 text-sm bg-blue-600 hover:bg-blue-700 text-white rounded {app.activeOperationId ? 'opacity-40 pointer-events-none' : ''}"
		>Update</a>
		<button
			onclick={handleDelete}
			disabled={actionPending || !!app.activeOperationId}
			class="px-3 py-1.5 text-sm bg-red-100 hover:bg-red-200 text-red-700 rounded disabled:opacity-40 disabled:cursor-not-allowed ml-auto"
		>Delete</button>
	</div>

	{#if actionError}
		<div class="mb-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">{actionError}</div>
	{/if}

	<!-- Tabs -->
	<div class="border-b border-gray-200 mb-6 flex gap-6">
		{#each [['overview', 'Overview'], ['events', 'Events'], ['history', 'History']] as [tab, label]}
			<button
				onclick={() => { activeTab = tab as typeof activeTab; if (tab === 'history') loadRevisions(); }}
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
		{#if revisions.length === 0}
			<p class="text-sm text-gray-500">No revisions loaded.</p>
		{:else}
			<table class="w-full text-sm border-collapse">
				<thead>
					<tr class="text-left text-xs text-gray-500 border-b border-gray-200">
						<th class="pb-2 pr-4">#</th>
						<th class="pb-2 pr-4">Result</th>
						<th class="pb-2 pr-4">Image</th>
						<th class="pb-2 pr-4">Replicas</th>
						<th class="pb-2 pr-4">Created</th>
						<th class="pb-2"></th>
					</tr>
				</thead>
				<tbody>
					{#each revisions as rev}
						<tr class="border-b border-gray-100 hover:bg-gray-50">
							<td class="py-2 pr-4 font-mono text-xs text-gray-500">{rev.number}</td>
							<td class="py-2 pr-4">
								<StatusBadge phase={rev.result} />
							</td>
							<td class="py-2 pr-4 font-mono text-xs truncate max-w-xs">{rev.spec.image}</td>
							<td class="py-2 pr-4">{rev.spec.replicas}</td>
							<td class="py-2 pr-4 text-xs text-gray-500">{new Date(rev.createdAt).toLocaleString()}</td>
							<td class="py-2">
								{#if rev.result === 'succeeded' && rev.id !== app.desiredRevision}
									<button
										onclick={() => handleRollback(rev.id)}
										disabled={actionPending || !!app!.activeOperationId}
										class="px-2 py-1 text-xs bg-gray-100 hover:bg-gray-200 text-gray-700 rounded disabled:opacity-40"
									>Roll back</button>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	{/if}
{/if}
