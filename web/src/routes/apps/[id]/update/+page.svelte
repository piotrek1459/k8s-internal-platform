<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { getApp, listRevisions, updateApp, ApiError } from '$lib/api.js';
	import type { Application, Revision } from '$lib/types.js';

	const id = $derived(page.params.id ?? '');

	let app = $state<Application | null>(null);
	let currentRevision = $state<Revision | null>(null);
	let loading = $state(true);
	let error = $state('');
	let submitting = $state(false);
	let submitError = $state('');

	// Form fields — loaded from current revision on mount
	let image = $state('');
	let replicas = $state(1);
	let envText = $state(''); // KEY=VALUE lines

	onMount(async () => {
		try {
			[app] = await Promise.all([getApp(id)]);
			const { items } = await listRevisions(id);
			currentRevision = items[0] ?? null;
			if (currentRevision) {
				image = currentRevision.spec.image;
				replicas = currentRevision.spec.replicas;
			}
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) goto('/login');
			else error = e instanceof Error ? e.message : 'Failed to load application';
		} finally {
			loading = false;
		}
	});

	async function handleSubmit(ev: SubmitEvent) {
		ev.preventDefault();
		if (!app || submitting) return;
		submitting = true;
		submitError = '';
		try {
			const body: Record<string, unknown> = { version: app.version };

			// Only include image if it has changed
			if (currentRevision && image !== currentRevision.spec.image) {
				body.image = image.trim() || undefined;
			}
			if (currentRevision && replicas !== currentRevision.spec.replicas) {
				body.replicas = replicas;
			}

			// Parse environment from KEY=VALUE lines
			const envLines = envText.trim().split('\n').filter(l => l.trim() !== '');
			if (envLines.length > 0) {
				const env: Record<string, string> = {};
				for (const line of envLines) {
					const eq = line.indexOf('=');
					if (eq > 0) {
						env[line.slice(0, eq).trim()] = line.slice(eq + 1);
					}
				}
				body.environment = env;
			}

			const res = await updateApp(id, crypto.randomUUID(), body);
			goto(`/apps/${id}/operation?op=${res.operationId}`);
		} catch (e) {
			submitError = e instanceof Error ? e.message : 'Update failed';
		} finally {
			submitting = false;
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
			<h1 class="text-xl font-semibold text-gray-900">Update {app.name}</h1>
			<p class="text-xs text-gray-400 font-mono mt-0.5">{app.id}</p>
		</div>
		<a href="/apps/{id}" class="text-sm text-gray-500 hover:text-gray-700">← Cancel</a>
	</div>

	<form onsubmit={handleSubmit} class="max-w-lg space-y-5">
		<div>
			<label class="block text-sm font-medium text-gray-700 mb-1" for="image">
				Image reference
			</label>
			<input
				id="image"
				type="text"
				bind:value={image}
				placeholder="registry/image:tag or @sha256:..."
				class="w-full px-3 py-2 border border-gray-300 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
			/>
			<p class="text-xs text-gray-400 mt-1">Leave unchanged to keep the currently deployed image.</p>
		</div>

		<div>
			<label class="block text-sm font-medium text-gray-700 mb-1" for="replicas">
				Replicas
			</label>
			<input
				id="replicas"
				type="number"
				bind:value={replicas}
				min="0"
				max="10"
				class="w-32 px-3 py-2 border border-gray-300 rounded text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
			/>
		</div>

		<div>
			<label class="block text-sm font-medium text-gray-700 mb-1" for="env">
				Environment variables <span class="text-gray-400 font-normal">(KEY=VALUE, one per line)</span>
			</label>
			<textarea
				id="env"
				bind:value={envText}
				rows="5"
				placeholder="LOG_LEVEL=info&#10;PORT=8080"
				class="w-full px-3 py-2 border border-gray-300 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
			></textarea>
			<p class="text-xs text-gray-400 mt-1">Replaces all environment variables. Leave empty to keep current values.</p>
		</div>

		{#if submitError}
			<div class="p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">{submitError}</div>
		{/if}

		<div class="flex gap-3 pt-2">
			<button
				type="submit"
				disabled={submitting || !!app.activeOperationId}
				class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded disabled:opacity-40 disabled:cursor-not-allowed"
			>
				{submitting ? 'Updating…' : 'Apply update'}
			</button>
			<a href="/apps/{id}" class="px-4 py-2 text-sm text-gray-600 hover:text-gray-800">Cancel</a>
		</div>

		{#if app.activeOperationId}
			<p class="text-xs text-amber-600">An operation is already in progress — updates are blocked.</p>
		{/if}
	</form>
{/if}
