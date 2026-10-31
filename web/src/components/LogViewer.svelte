<script lang="ts">
	interface Props { lines: string[] }
	let { lines }: Props = $props();
</script>

<pre class="bg-gray-950 text-gray-100 text-xs font-mono p-4 rounded overflow-auto max-h-96 leading-relaxed">{#each lines as line}{line}
{/each}</pre>
