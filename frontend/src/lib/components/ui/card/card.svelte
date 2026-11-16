<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLAttributes } from "svelte/elements";
  import { cn } from "$lib/utils";
  type Props = HTMLAttributes<HTMLDivElement> & { children?: Snippet };
  let { class: className, children, ...rest }: Props = $props();
</script>

<div data-slot="card" class={cn("rounded-2xl border border-border/80 bg-card text-card-foreground shadow-[0_8px_30px_rgba(0,0,0,0.035)]", className)} {...rest}>
  {#if children}{@render children()}{/if}
</div>
