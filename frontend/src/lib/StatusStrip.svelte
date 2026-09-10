<!-- The status strip. 22px of the chrome budget, and the only place that can
     say the daemon is gone: a rail with no programs in it looks exactly like a
     rail whose daemon died. Healthy is the absence of colour (section 11), so
     connected is a hollow ring and only trouble gets a hue. -->
<script lang="ts">
  interface Props {
    connected: boolean;
    socket: string;
    programCount: number;
    build: Record<string, string> | null;
    lastRead: string;
  }

  let { connected, socket, programCount, build, lastRead }: Props = $props();
</script>

<footer class="strip">
  <span class="dot" class:bad={!connected} aria-hidden="true"></span>
  <span>
    {#if connected}
      rig, {programCount} program{programCount === 1 ? "" : "s"}
    {:else}
      detached
    {/if}
  </span>
  <span class="sock" title={socket}>{socket}</span>
  <span class="r">
    {#if build}{build.version} {build.wire}{/if}
    {#if lastRead}&nbsp;&middot; read {lastRead}{/if}
  </span>
</footer>

<style>
  .sock {
    font-family: var(--mono);
    color: var(--fg-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
</style>
