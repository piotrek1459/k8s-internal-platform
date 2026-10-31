<script lang="ts">
	import { page } from '$app/state';
	import { session } from '$lib/auth.js';

	const navItems = [
		{ href: '/apps', label: 'Applications', icon: '⬡' },
		{ href: '/secrets', label: 'Secrets', icon: '🔑' },
		{ href: '/credentials', label: 'Registries', icon: '📦' }
	];
</script>

<nav class="flex flex-col w-48 min-h-screen bg-gray-900 text-gray-100 px-3 py-6 gap-1 shrink-0">
	<a href="/apps" class="text-white font-semibold text-lg mb-6 px-2">Platform</a>

	{#each navItems as item}
		<a
			href={item.href}
			class="flex items-center gap-2 px-2 py-2 rounded text-sm transition-colors
				{page.url.pathname.startsWith(item.href)
				? 'bg-gray-700 text-white'
				: 'text-gray-400 hover:text-white hover:bg-gray-800'}"
		>
			<span class="w-4 text-center text-xs">{item.icon}</span>
			{item.label}
		</a>
	{/each}

	<div class="mt-auto">
		{#if $session.administratorId}
			<button
				onclick={() => session.logout()}
				class="w-full text-left px-2 py-2 rounded text-sm text-gray-400 hover:text-white hover:bg-gray-800 transition-colors"
			>
				Sign out
			</button>
		{/if}
	</div>
</nav>
