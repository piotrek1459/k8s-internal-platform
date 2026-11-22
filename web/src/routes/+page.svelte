<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { getSetupStatus } from '$lib/api.js';
	let error = $state('');
	onMount(async () => {
		try { await goto((await getSetupStatus()).setupAvailable ? '/setup' : '/apps'); }
		catch { error = 'The platform could not be reached. Refresh to retry.'; }
	});
</script>

<p>{error || 'Opening platform…'}</p>
