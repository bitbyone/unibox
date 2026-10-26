<script setup lang="ts">
import { onMounted, ref } from 'vue'

// The three flows of a task, each a dot travelling along its path: your
// files into the sandbox, a request out through the proxy where the token
// is added, and a port back to localhost. Someone who asked for less motion
// gets the still picture, which says the same.
const animate = ref(true)
onMounted(() => {
  animate.value = !window.matchMedia('(prefers-reduced-motion: reduce)').matches
})
</script>

<template>
  <figure class="arch">
    <div class="scroll">
    <svg viewBox="0 0 920 410" role="img" aria-labelledby="arch-title arch-desc">
      <title id="arch-title">What unibox run does</title>
      <desc id="arch-desc">
        Your repository is mounted into a sandbox, a micro-VM. The agent inside sends requests with
        placeholders; sbx's proxy on your machine swaps them for the real token and lets them out only
        to the hosts the env allows. A port the sandbox opens appears on localhost.
      </desc>

      <defs>
        <marker id="arrow-files" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
          <path d="M0 0 L10 5 L0 10 z" class="head-files" />
        </marker>
        <marker id="arrow-token" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
          <path d="M0 0 L10 5 L0 10 z" class="head-token" />
        </marker>
        <marker id="arrow-port" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
          <path d="M0 0 L10 5 L0 10 z" class="head-port" />
        </marker>
        <path id="p-mount" d="M260 155 H660" />
        <path id="p-token" d="M260 245 H340" />
        <path id="p-placeholder" d="M660 245 H580" />
        <path id="p-out" d="M400 220 V162 A7 7 0 0 0 400 148 V36 H560" />
        <path id="p-port" d="M660 335 H260" />
      </defs>

      <!-- the internet, only the hosts the env names -->
      <g class="cloud">
        <rect x="560" y="14" width="340" height="44" rx="22" />
        <text x="730" y="35" text-anchor="middle" class="t-strong">github.com · api.anthropic.com · your registry</text>
        <text x="730" y="50" text-anchor="middle" class="t-dim">only the hosts the env allows</text>
      </g>

      <!-- columns -->
      <g class="column">
        <rect x="20" y="84" width="240" height="306" rx="14" />
        <text x="140" y="110" text-anchor="middle" class="t-head">Your machine</text>
      </g>
      <g class="column column-mid">
        <rect x="340" y="84" width="240" height="306" rx="14" />
        <text x="460" y="110" text-anchor="middle" class="t-head">unibox drives sbx</text>
      </g>
      <g class="column column-vm">
        <rect x="660" y="84" width="240" height="306" rx="14" />
        <text x="780" y="110" text-anchor="middle" class="t-head">Sandbox, a micro-VM</text>
      </g>

      <!-- row 1: files -->
      <g class="card">
        <rect x="36" y="130" width="208" height="50" rx="10" />
        <text x="50" y="151" class="t-mono">~/code/api</text>
        <text x="50" y="168" class="t-dim">your checkout</text>
      </g>
      <g class="card">
        <rect x="668" y="130" width="224" height="50" rx="10" />
        <text x="682" y="151" class="t-mono">/work/login/api</text>
        <text x="682" y="168" class="t-dim">the same files, mounted live</text>
      </g>
      <use href="#p-mount" class="flow flow-files" marker-end="url(#arrow-files)" />

      <!-- row 2: tokens -->
      <g class="card">
        <rect x="36" y="220" width="208" height="50" rx="10" />
        <text x="50" y="241" class="t-strong">keychain</text>
        <text x="50" y="258" class="t-dim">your tokens, stored by sbx</text>
      </g>
      <g class="card card-proxy">
        <rect x="356" y="220" width="208" height="50" rx="10" />
        <text x="370" y="241" class="t-strong">sbx proxy</text>
        <text x="370" y="258" class="t-dim">puts the token into the request</text>
      </g>
      <g class="card">
        <rect x="668" y="220" width="224" height="50" rx="10" />
        <text x="682" y="241" class="t-strong">the agent</text>
        <text x="682" y="258" class="t-dim">claude · codex · copilot · opencode</text>
      </g>
      <use href="#p-token" class="flow flow-token" marker-end="url(#arrow-token)" />
      <use href="#p-placeholder" class="flow flow-token dashed" marker-end="url(#arrow-token)" />
      <use href="#p-out" class="flow flow-token" marker-end="url(#arrow-token)" />
      <text x="300" y="237" text-anchor="middle" class="t-flow">token</text>
      <text x="620" y="237" text-anchor="middle" class="t-flow">placeholder</text>
      <text x="412" y="74" class="t-flow">the request, with the token in it</text>

      <!-- row 3: ports -->
      <g class="card">
        <rect x="36" y="310" width="208" height="50" rx="10" />
        <text x="50" y="331" class="t-mono">localhost:5173</text>
        <text x="50" y="348" class="t-dim">the dev server, in your browser</text>
      </g>
      <g class="card">
        <rect x="668" y="310" width="224" height="50" rx="10" />
        <text x="682" y="331" class="t-mono">:5173</text>
        <text x="682" y="348" class="t-dim">a port named vite in the env</text>
      </g>
      <use href="#p-port" class="flow flow-port" marker-end="url(#arrow-port)" />

      <!-- the one rule -->
      <text x="780" y="384" text-anchor="middle" class="t-rule">no token is ever inside</text>

      <!-- the dots -->
      <g v-if="animate">
        <circle r="5" class="dot dot-files">
          <animateMotion dur="3.2s" repeatCount="indefinite" keyPoints="0;1" keyTimes="0;1" calcMode="linear">
            <mpath href="#p-mount" />
          </animateMotion>
        </circle>
        <circle r="5" class="dot dot-token">
          <animateMotion dur="4s" repeatCount="indefinite" keyPoints="0;1;1" keyTimes="0;0.25;1" calcMode="linear">
            <mpath href="#p-placeholder" />
          </animateMotion>
          <animate attributeName="opacity" dur="4s" repeatCount="indefinite" values="1;1;0;0" keyTimes="0;0.25;0.26;1" />
        </circle>
        <circle r="5" class="dot dot-token">
          <animateMotion dur="4s" repeatCount="indefinite" keyPoints="0;0;1;1" keyTimes="0;0.2;0.45;1" calcMode="linear">
            <mpath href="#p-token" />
          </animateMotion>
          <animate attributeName="opacity" dur="4s" repeatCount="indefinite" values="0;1;1;0;0" keyTimes="0;0.2;0.45;0.46;1" />
        </circle>
        <circle r="5" class="dot dot-token">
          <animateMotion dur="4s" repeatCount="indefinite" keyPoints="0;0;1;1" keyTimes="0;0.45;0.9;1" calcMode="linear">
            <mpath href="#p-out" />
          </animateMotion>
          <animate attributeName="opacity" dur="4s" repeatCount="indefinite" values="0;1;1;0" keyTimes="0;0.45;0.9;1" />
        </circle>
        <circle r="5" class="dot dot-port">
          <animateMotion dur="3.6s" repeatCount="indefinite" begin="1s">
            <mpath href="#p-port" />
          </animateMotion>
        </circle>
      </g>

      <!-- the pills sit over the dots -->
      <g class="pill">
        <rect x="450" y="142" width="84" height="26" rx="13" class="pill-bg" />
        <rect x="450" y="142" width="84" height="26" rx="13" />
        <text x="492" y="159" text-anchor="middle" class="t-pill">mount</text>
      </g>
      <g class="pill">
        <rect x="392" y="322" width="136" height="26" rx="13" class="pill-bg" />
        <rect x="392" y="322" width="136" height="26" rx="13" />
        <text x="460" y="339" text-anchor="middle" class="t-pill">unibox port on vite</text>
      </g>
    </svg>
    </div>
    <p class="hint">Scroll the picture sideways on a small screen.</p>

    <figcaption>
      <span><i class="k k-files" /> your files, mounted into the sandbox</span>
      <span><i class="k k-token" /> a request: the placeholder leaves, the token is added on your machine</span>
      <span><i class="k k-port" /> a port, back to localhost</span>
    </figcaption>
  </figure>
</template>

<style scoped>
.arch { margin: 0; }
/* On a narrow screen the picture keeps a readable size and scrolls sideways. */
.scroll { overflow-x: auto; -webkit-overflow-scrolling: touch; }
svg {
  display: block;
  width: 100%;
  min-width: 720px;
  height: auto;
  font-family: var(--vp-font-family-base);
}
.cloud rect { fill: var(--ub-code-bg); stroke: var(--ub-frame); }
.column rect { fill: var(--vp-c-bg); stroke: var(--ub-frame); stroke-width: 1.5; }
.column-mid rect { fill: var(--ub-code-bg); }
.column-vm rect { stroke: var(--ub-brand); stroke-dasharray: 6 5; }
.card rect { fill: var(--vp-c-bg); stroke: var(--ub-frame); }
.card-proxy rect { stroke: var(--ub-accent); }
.pill rect { fill: var(--ub-brand-soft); }
.pill rect.pill-bg { fill: var(--ub-code-bg); }
.t-head { font-size: 13px; font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; fill: var(--vp-c-text-2); }
.t-strong { font-size: 13.5px; font-weight: 600; fill: var(--vp-c-text-1); }
.t-mono { font-family: var(--vp-font-family-mono); font-size: 13px; font-weight: 600; fill: var(--vp-c-text-1); }
.t-dim { font-size: 11.5px; fill: var(--vp-c-text-2); }
.t-pill { font-family: var(--vp-font-family-mono); font-size: 11.5px; font-weight: 600; fill: var(--ub-brand); }
.t-flow { font-size: 11px; fill: var(--vp-c-text-3); }
.t-rule { font-size: 12.5px; font-weight: 600; fill: var(--ub-brand); }
.flow { fill: none; stroke-width: 1.75; }
.flow-files { stroke: var(--ub-brand); }
.flow-token { stroke: var(--ub-accent); }
.flow-port { stroke: var(--vp-c-text-3); }
.dashed { stroke-dasharray: 4 4; }
.head-files { fill: var(--ub-brand); }
.head-token { fill: var(--ub-accent); }
.head-port { fill: var(--vp-c-text-3); }
.dot-files { fill: var(--ub-brand); }
.dot-token { fill: var(--ub-accent); }
.dot-port { fill: var(--vp-c-text-2); }

.hint { display: none; margin: 8px 0 0; font-size: 12px; color: var(--vp-c-text-3); }
@media (max-width: 760px) { .hint { display: block; } }
figcaption {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 10px 28px;
  margin-top: 20px;
  font-size: 13.5px;
  color: var(--vp-c-text-2);
}
figcaption span { display: inline-flex; align-items: flex-start; gap: 8px; text-align: left; line-height: 1.5; }
.k { width: 10px; height: 10px; border-radius: 50%; flex: none; margin-top: 5px; }
.k-files { background: var(--ub-brand); }
.k-token { background: var(--ub-accent); }
.k-port { background: var(--vp-c-text-3); }
</style>
