<script lang="ts">
	import { inspectImage, listSecrets, listCredentials, createApp, ApiError } from '$lib/api.js';
	import OperationProgress from '$components/OperationProgress.svelte';
	import type { Secret, RegistryCredential, ImageInspectResult } from '$lib/types.js';

	// ── Step state ──────────────────────────────────────────────────────────────
	let step = $state(1);

	// Step 1 – Image
	let image = $state('');
	let credentialId = $state('');
	let imageError = $state('');
	let inspecting = $state(false);
	let inspected = $state<ImageInspectResult | null>(null);
	let credentials = $state<RegistryCredential[]>([]);

	// Step 2 – Application
	let name = $state('');
	let containerPort = $state(8080);
	let readinessPath = $state('/healthz');

	// Step 3 – Access
	let exposureMode = $state<'internal' | 'private' | 'public'>('internal');

	// Step 4 – Configuration
	let envEntries = $state<[string, string][]>([['', '']]);
	let secrets = $state<Secret[]>([]);
	let secretRef = $state<{ envVar: string; secretId: string; version: number } | null>(null);

	// Step 6 – Deploy
	let operationId = $state('');
	let submitting = $state(false);
	let submitError = $state('');

	// ── Step 1: inspect image ────────────────────────────────────────────────────
	async function handleInspect() {
		imageError = '';
		inspecting = true;
		try {
			inspected = await inspectImage(image.trim(), credentialId || undefined);
			if (inspected.suggestedPort) containerPort = inspected.suggestedPort;
			// Suggest a name from the image path.
			if (!name) {
				const parts = image.split('/');
				name = parts[parts.length - 1].split(':')[0].replace(/[^a-z0-9-]/g, '-');
			}
		} catch (e) {
			if (e instanceof ApiError) imageError = e.message;
			else imageError = 'Image inspection failed.';
		} finally {
			inspecting = false;
		}
	}

	async function loadCredentials() {
		try {
			const res = await listCredentials();
			credentials = res.items;
		} catch { /* ignore if registry endpoint not yet available */ }
	}

	async function loadSecrets() {
		try {
			const res = await listSecrets();
			secrets = res.items;
		} catch { /* ignore */ }
	}

	// ── Step navigation ──────────────────────────────────────────────────────────
	async function goToStep(n: number) {
		if (n === 1 && step === 1) { loadCredentials(); return; }
		if (n === 4) loadSecrets();
		step = n;
	}

	$effect(() => { if (step === 1) loadCredentials(); });
	$effect(() => { if (step === 4) loadSecrets(); });

	// ── Final submission ─────────────────────────────────────────────────────────
	async function deploy() {
		submitError = '';
		submitting = true;
		const env: Record<string, string> = {};
		for (const [k, v] of envEntries) {
			if (k.trim()) env[k.trim()] = v;
		}
		try {
			const key = `create-${name}-${Date.now()}`;
			const res = await createApp(key, {
				name: name.trim(),
				image: image.trim(),
				registryCredentialId: credentialId || undefined,
				containerPort,
				profile: 'http',
				environment: Object.keys(env).length > 0 ? env : undefined,
				health: readinessPath ? { readinessPath } : undefined,
				exposure: { mode: exposureMode }
			});
			operationId = res.operationId;
			step = 6;
		} catch (e) {
			submitError = e instanceof ApiError ? e.message : 'Deployment failed.';
		} finally {
			submitting = false;
		}
	}

	const STEP_LABELS = ['Image', 'Application', 'Access', 'Configuration', 'Review', 'Deploy'];
</script>

<!-- Progress indicator -->
<div class="mb-8">
	<div class="flex items-center gap-2">
		{#each STEP_LABELS as label, i}
			<button
				onclick={() => i + 1 < step && goToStep(i + 1)}
				disabled={i + 1 > step}
				class="flex items-center gap-1 text-xs {i + 1 === step
					? 'text-blue-700 font-semibold'
					: i + 1 < step
						? 'text-gray-500 hover:text-gray-700 cursor-pointer'
						: 'text-gray-300 cursor-default'}"
			>
				<span class="w-5 h-5 rounded-full border flex items-center justify-center
					{i + 1 === step ? 'border-blue-600 bg-blue-50' : i + 1 < step ? 'border-green-500 bg-green-50' : 'border-gray-200'}">
					{i + 1 < step ? '✓' : i + 1}
				</span>
				{label}
			</button>
			{#if i < STEP_LABELS.length - 1}
				<span class="text-gray-200 text-xs">›</span>
			{/if}
		{/each}
	</div>
</div>

<div class="max-w-xl">

<!-- Step 1: Image -->
{#if step === 1}
	<h2 class="text-lg font-semibold text-gray-900 mb-4">Container image</h2>
	{#if imageError}
		<div class="mb-3 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">{imageError}</div>
	{/if}
	<div class="space-y-3">
		<div>
			<label for="image" class="block text-sm font-medium text-gray-700 mb-1">Image reference</label>
			<input id="image" type="text" bind:value={image} placeholder="ghcr.io/example/app:v1.0.0"
				class="w-full border border-gray-300 rounded px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500" />
		</div>
		{#if credentials.length > 0}
			<div>
				<label for="cred" class="block text-sm font-medium text-gray-700 mb-1">Registry credential (optional)</label>
				<select id="cred" bind:value={credentialId}
					class="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500">
					<option value="">None (public image)</option>
					{#each credentials as c}
						<option value={c.id}>{c.label} ({c.registry})</option>
					{/each}
				</select>
			</div>
		{/if}
		<div class="flex gap-2">
			<button onclick={handleInspect} disabled={!image.trim() || inspecting}
				class="px-4 py-2 bg-gray-100 hover:bg-gray-200 disabled:opacity-50 text-sm rounded transition-colors">
				{inspecting ? 'Inspecting…' : 'Inspect image'}
			</button>
			{#if inspected}
				<button onclick={() => goToStep(2)}
					class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded transition-colors">
					Next →
				</button>
			{/if}
		</div>
		{#if inspected}
			<div class="mt-2 p-3 bg-green-50 border border-green-200 rounded text-xs text-green-800 space-y-1">
				<p><strong>Digest:</strong> {inspected.digest.slice(0, 24)}…</p>
				<p><strong>Architectures:</strong> {inspected.architectures.join(', ')}</p>
				{#if inspected.suggestedPort}
					<p><strong>Suggested port:</strong> {inspected.suggestedPort}</p>
				{/if}
			</div>
		{/if}
	</div>

<!-- Step 2: Application -->
{:else if step === 2}
	<h2 class="text-lg font-semibold text-gray-900 mb-4">Application settings</h2>
	<div class="space-y-3">
		<div>
			<label for="name" class="block text-sm font-medium text-gray-700 mb-1">Name</label>
			<input id="name" type="text" bind:value={name} placeholder="my-app"
				class="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
			<p class="text-xs text-gray-400 mt-1">Lowercase letters, numbers and hyphens only.</p>
		</div>
		<div>
			<label for="port" class="block text-sm font-medium text-gray-700 mb-1">Container port</label>
			<input id="port" type="number" bind:value={containerPort} min="1" max="65535"
				class="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
		</div>
		<div>
			<label for="health" class="block text-sm font-medium text-gray-700 mb-1">Readiness path</label>
			<input id="health" type="text" bind:value={readinessPath} placeholder="/healthz"
				class="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" />
		</div>
		<div class="flex gap-2 pt-1">
			<button onclick={() => goToStep(1)} class="px-4 py-2 border border-gray-300 text-sm rounded hover:bg-gray-50 transition-colors">← Back</button>
			<button onclick={() => goToStep(3)} disabled={!name.trim()}
				class="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white text-sm rounded transition-colors">Next →</button>
		</div>
	</div>

<!-- Step 3: Access -->
{:else if step === 3}
	<h2 class="text-lg font-semibold text-gray-900 mb-4">Access</h2>
	<div class="space-y-2">
		{#each [['internal', 'Internal (cluster-only, no ingress route)'], ['private', 'Private LAN (accessible on your local network)']] as [value, label]}
			<label class="flex items-start gap-3 p-3 border rounded cursor-pointer hover:bg-gray-50 transition-colors
				{exposureMode === value ? 'border-blue-400 bg-blue-50' : 'border-gray-200'}">
				<input type="radio" name="exposure" {value} bind:group={exposureMode} class="mt-0.5" />
				<span class="text-sm">{label}</span>
			</label>
		{/each}
		<label class="flex items-start gap-3 p-3 border rounded opacity-50 cursor-not-allowed border-gray-200">
			<input type="radio" name="exposure" value="public" disabled />
			<span class="text-sm">Public (requires networking configuration — available in a future release)</span>
		</label>
	</div>
	<div class="flex gap-2 mt-4">
		<button onclick={() => goToStep(2)} class="px-4 py-2 border border-gray-300 text-sm rounded hover:bg-gray-50 transition-colors">← Back</button>
		<button onclick={() => goToStep(4)} class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded transition-colors">Next →</button>
	</div>

<!-- Step 4: Configuration -->
{:else if step === 4}
	<h2 class="text-lg font-semibold text-gray-900 mb-4">Configuration</h2>
	<div class="space-y-4">
		<div>
			<p class="text-sm font-medium text-gray-700 mb-2">Environment variables</p>
			{#each envEntries as _, i}
				<div class="flex gap-2 mb-2">
					<input type="text" bind:value={envEntries[i][0]} placeholder="KEY"
						class="w-1/3 border border-gray-300 rounded px-2 py-1.5 text-xs font-mono focus:outline-none focus:ring-2 focus:ring-blue-500" />
					<input type="text" bind:value={envEntries[i][1]} placeholder="value"
						class="flex-1 border border-gray-300 rounded px-2 py-1.5 text-xs font-mono focus:outline-none focus:ring-2 focus:ring-blue-500" />
					<button onclick={() => envEntries = envEntries.filter((_, j) => j !== i)}
						class="text-gray-400 hover:text-red-500 text-xs px-1">✕</button>
				</div>
			{/each}
			<button onclick={() => envEntries = [...envEntries, ['', '']]}
				class="text-xs text-blue-600 hover:underline mt-1">+ Add variable</button>
		</div>
		{#if secrets.length > 0}
			<div>
				<p class="text-sm font-medium text-gray-700 mb-2">Secret reference (optional)</p>
				<p class="text-xs text-gray-500 mb-2">Inject a secret value as an environment variable.</p>
				<div class="flex gap-2">
					<input type="text" placeholder="ENV_VAR_NAME"
						bind:value={secretRef!.envVar}
						class="w-1/3 border border-gray-300 rounded px-2 py-1.5 text-xs font-mono focus:outline-none focus:ring-2 focus:ring-blue-500" />
					<select bind:value={secretRef!.secretId}
						class="flex-1 border border-gray-300 rounded px-2 py-1.5 text-xs focus:outline-none focus:ring-2 focus:ring-blue-500">
						<option value="">Select secret…</option>
						{#each secrets as s}
							<option value={s.id}>{s.name}</option>
						{/each}
					</select>
				</div>
			</div>
		{/if}
	</div>
	<div class="flex gap-2 mt-4">
		<button onclick={() => goToStep(3)} class="px-4 py-2 border border-gray-300 text-sm rounded hover:bg-gray-50 transition-colors">← Back</button>
		<button onclick={() => goToStep(5)} class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded transition-colors">Next →</button>
	</div>

<!-- Step 5: Review -->
{:else if step === 5}
	<h2 class="text-lg font-semibold text-gray-900 mb-4">Review</h2>
	<dl class="divide-y divide-gray-100 text-sm">
		{#each [['Image', image], ['Name', name], ['Port', String(containerPort)], ['Readiness', readinessPath], ['Access', exposureMode]] as [k, v]}
			<div class="flex py-2">
				<dt class="w-28 text-gray-500">{k}</dt>
				<dd class="font-medium text-gray-900 break-all">{v}</dd>
			</div>
		{/each}
	</dl>
	{#if submitError}
		<div class="mt-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">{submitError}</div>
	{/if}
	<div class="flex gap-2 mt-6">
		<button onclick={() => goToStep(4)} class="px-4 py-2 border border-gray-300 text-sm rounded hover:bg-gray-50 transition-colors">← Back</button>
		<button onclick={deploy} disabled={submitting}
			class="px-6 py-2 bg-green-600 hover:bg-green-700 disabled:opacity-50 text-white font-medium text-sm rounded transition-colors">
			{submitting ? 'Deploying…' : 'Deploy'}
		</button>
	</div>

<!-- Step 6: Deploy (operation progress) -->
{:else if step === 6}
	<h2 class="text-lg font-semibold text-gray-900 mb-4">Deploying</h2>
	{#if operationId}
		<OperationProgress {operationId} />
		<div class="mt-6 flex gap-3">
			<a href="/apps" class="text-sm text-blue-600 hover:underline">← All applications</a>
		</div>
	{/if}
{/if}

</div>
