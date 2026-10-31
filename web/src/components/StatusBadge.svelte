<script lang="ts">
	const PHASE_STYLES: Record<string, string> = {
		ready: 'bg-green-100 text-green-800',
		deploying: 'bg-blue-100 text-blue-800',
		updating: 'bg-blue-100 text-blue-800',
		degraded: 'bg-yellow-100 text-yellow-800',
		failed: 'bg-red-100 text-red-800',
		stopped: 'bg-gray-100 text-gray-600',
		unknown: 'bg-gray-100 text-gray-500'
	};

	interface Props { phase: string }
	let { phase }: Props = $props();
	const style = $derived(PHASE_STYLES[phase] ?? PHASE_STYLES.unknown);
</script>

<span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium {style}">
	{phase || 'unknown'}
</span>
