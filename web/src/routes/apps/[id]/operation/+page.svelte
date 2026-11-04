<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import OperationProgress from '$components/OperationProgress.svelte';
	import { requireAuth } from '$lib/auth.js';

	const appId = $derived(page.params.id);
	const operationId = $derived(page.url.searchParams.get('op') ?? '');

	onMount(async () => {
		await requireAuth();
		if (!operationId) {
			goto(`/apps/${appId}`);
		}
	});
</script>

<div class="p-6 max-w-2xl">
	<a href="/apps/{appId}" class="text-sm text-gray-400 hover:text-white mb-4 inline-block">
		← Back to application
	</a>

	<h1 class="text-xl font-semibold text-white mb-6">Operation progress</h1>

	{#if operationId}
		<OperationProgress {operationId} />
	{:else}
		<p class="text-gray-400">No operation ID provided.</p>
	{/if}
</div>
