<script lang="ts">
	import { getAppLogs, getAppEvents, getAppDrift } from '$lib/api.js';
	import type { ApplicationLogs, ApplicationEvents, ApplicationDrift } from '$lib/types.js';
	import LogViewer from './LogViewer.svelte';
	let { appId }: { appId: string } = $props();
	let pod = $state('');
	let previous = $state(false);
	let tailLines = $state(100);
	let sinceSeconds = $state(3600);
	let logs = $state<ApplicationLogs | null>(null);
	let events = $state<ApplicationEvents | null>(null);
	let drift = $state<ApplicationDrift | null>(null);
	let pending = $state(false);
	let error = $state('');

	async function read(kind: 'logs' | 'events' | 'drift') {
		if (pending) return;
		pending = true; error = '';
		try {
			if (kind === 'logs') logs = await getAppLogs(appId, pod, previous, tailLines, sinceSeconds);
			if (kind === 'events') events = await getAppEvents(appId);
			if (kind === 'drift') drift = await getAppDrift(appId);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Diagnostics could not be read';
			if (kind === 'logs') logs = null;
			if (kind === 'events') events = null;
			if (kind === 'drift') drift = null;
		} finally { pending = false; }
	}
</script>

<div class="space-y-6">
	<p class="text-sm text-gray-500">Diagnostics are read on request. Known managed Secret values are masked in logs and events. Configuration inspection reports field differences without changing resources.</p>
	{#if error}<p role="alert" class="text-sm text-red-700">{error}</p>{/if}
	<section aria-label="Application logs" class="space-y-3">
		<h2 class="font-medium">Application logs</h2>
		<div class="flex flex-wrap items-end gap-3 text-sm">
			<label>Pod name (optional)<input aria-label="Log Pod name" bind:value={pod} class="block border rounded p-2" placeholder="Newest active Pod" /></label>
			<label>Recent lines<select aria-label="Recent log lines" bind:value={tailLines} class="block border rounded p-2"><option value={100}>100</option><option value={500}>500</option></select></label>
			<label>Time range<select aria-label="Log time range" bind:value={sinceSeconds} class="block border rounded p-2"><option value={3600}>Last hour</option><option value={86400}>Last day</option></select></label>
			<label><input type="checkbox" bind:checked={previous} /> Previous container</label>
			<button disabled={pending} onclick={() => read('logs')} class="px-3 py-2 border rounded disabled:opacity-50">Read logs</button>
		</div>
		{#if logs}
			<p class="text-xs text-gray-500">{logs.pod} · {logs.container} · {new Date(logs.observedAt).toLocaleString()}</p>
			{#if logs.truncated}<p class="text-sm text-amber-700">Log output was truncated.</p>{/if}
			{#if logs.text}<LogViewer lines={logs.text.split('\n')} />{:else}<p class="text-sm text-gray-500">No log output in the selected range.</p>{/if}
		{/if}
	</section>
	<section aria-label="Workload events" class="space-y-3">
		<h2 class="font-medium">Kubernetes events</h2>
		<button disabled={pending} onclick={() => read('events')} class="px-3 py-2 border rounded disabled:opacity-50">Read events</button>
		{#if events}
			{#if events.truncated}<p class="text-sm text-amber-700">Event results were truncated; some retained events may be omitted.</p>{/if}
			{#if events.items.length === 0}<p class="text-sm text-gray-500">No retained events for current owned resources.</p>{/if}
			<ul class="space-y-2 text-sm">{#each events.items as event}<li class="border rounded p-3"><p class="font-medium">{event.type} · {event.reason} · {event.resource}</p><p class="whitespace-pre-wrap break-words">{event.message}</p><p class="text-xs text-gray-500">{new Date(event.at).toLocaleString()} · count {event.count}</p></li>{/each}</ul>
		{/if}
	</section>
	<section aria-label="Configuration drift" class="space-y-3">
		<h2 class="font-medium">Configuration drift</h2>
		<button disabled={pending} onclick={() => read('drift')} class="px-3 py-2 border rounded disabled:opacity-50">Inspect drift</button>
		{#if drift}
			<p class="text-sm font-medium">{drift.state === 'in_sync' ? 'No owned configuration differences detected.' : 'Configuration drift detected.'}</p>
			<p class="text-xs text-gray-500">Revision {drift.revisionId} · version {drift.applicationVersion} · {new Date(drift.observedAt).toLocaleString()}</p>
			{#if drift.operationActive}<p class="text-sm text-amber-700">An operation is active; live state may still be changing. Inspect again after it completes.</p>{/if}
			<ul class="space-y-2 text-sm">{#each drift.differences as difference}<li class="border rounded p-3"><p>{difference.resource} · {difference.change}</p><code class="break-all">{difference.field}</code></li>{/each}</ul>
			<p class="text-sm text-gray-500">Reapply is an explicit application action. Readiness and traffic health are reported separately.</p>
		{/if}
	</section>
</div>
