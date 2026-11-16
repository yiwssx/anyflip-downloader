<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLButtonAttributes } from "svelte/elements";
  import { tv, type VariantProps } from "tailwind-variants";
  import { cn } from "$lib/utils";

  const buttonVariants = tv({
    base: "inline-flex min-h-11 shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-xl px-4 text-sm font-semibold transition-colors disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:ring-offset-2",
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/90",
        secondary: "bg-secondary text-secondary-foreground hover:bg-secondary/80",
        outline: "border border-border bg-card text-foreground hover:bg-accent",
        ghost: "text-foreground hover:bg-accent",
        destructive: "bg-destructive text-white hover:bg-destructive/90",
      },
      size: {
        default: "h-11 px-5",
        sm: "h-11 px-3.5 text-sm",
        icon: "size-11 p-0",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  });

  type Variant = VariantProps<typeof buttonVariants>["variant"];
  type Size = VariantProps<typeof buttonVariants>["size"];
  type Props = HTMLButtonAttributes & {
    variant?: Variant;
    size?: Size;
    children?: Snippet;
  };
  let { variant = "default", size = "default", class: className, children, ...rest }: Props = $props();
</script>

<button data-slot="button" class={cn(buttonVariants({ variant, size }), className)} {...rest}>
  {#if children}{@render children()}{/if}
</button>
