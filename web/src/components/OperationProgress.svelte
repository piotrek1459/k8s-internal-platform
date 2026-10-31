<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { getOperation } from '$lib/api.js';
	import type { Operation } from '$lib/types.js';

	interface Props { operationId: string }
	let { operationId }: Props = $props();

	let op = $state<Operation | null>(null);
	let error = $state('');
	let interval: ReturnType<typeof setInterval>;

	const TERMINAL = new Set(['succeeded', 'failed', 'needs_attention']);

	async function poll() {
		try {
			op = await getOperation(operationId);
			if (TERMINAL.has(op.state)) clearInterval(interval);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load operation';
			clearInterval(interval);
		}
	}

	onMount(() => {
		poll();
		interval = setInterval(poll, 2000);
	});
	onDestroy(() => clearInterval(interval));
</script>

<div class="space-y-2">
	{#if error}
		<p class="text-red-600 text-sm">{error}</p>
	{:else if !op}
		<p class="text-gray-500 text-sm">Loading operation…</p>
	{:else}
		<div class="flex items-center gap-2 text-sm font-medium">
			<span>State:</span>
			<span class:text-green-700={op.state === 'succeeded'}
				class:text-red-700={op.state === 'failed' || op.state === 'needs_attention'}
				class:text-blue-700={op.state === 'running' || op.state === 'queued'}>
				{op.state}
			</span>
		</div>
		<ol class="border-l border-gray-200 pl-4 space-y-2 mt-2">
			{#each op.events as ev}
				<li class="text-xs text-gray-600">
					<span class="font-medium text-gray-800">{ev.stage}</span>
					<span class="mx-1">·</span>
					{ev.message}
				</li>
			{/each}
		</ol>
	{/if}
</div>
