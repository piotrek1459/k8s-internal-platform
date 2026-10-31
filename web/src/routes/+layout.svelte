<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';
	import Sidebar from '$components/Sidebar.svelte';

	interface Props { children: import('svelte').Snippet }
	let { children }: Props = $props();

	// Routes that don't show the sidebar (unauthenticated).
	const bare = ['/setup', '/login'];
	const showSidebar = $derived(!bare.some(p => page.url.pathname.startsWith(p)));
</script>

{#if showSidebar}
	<div class="flex min-h-screen">
		<Sidebar />
		<main class="flex-1 p-8 bg-gray-50 overflow-auto">
			{@render children()}
		</main>
	</div>
{:else}
	<main class="min-h-screen bg-gray-50 flex items-center justify-center">
		{@render children()}
	</main>
{/if}
