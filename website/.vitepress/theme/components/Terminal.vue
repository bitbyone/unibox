<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

export interface TermLine {
  kind: 'cmd' | 'out' | 'ok' | 'dim' | 'blank'
  text?: string
}

const props = defineProps<{ lines: TermLine[]; title?: string }>()

// Commands are typed, output appears line by line, once the window has
// scrolled into view. Someone who asked for less motion, or whose browser
// has no IntersectionObserver, sees the whole session at once.
const shown = ref<{ kind: TermLine['kind']; text: string; typing: boolean }[]>([])
const done = ref(false)
const root = ref<HTMLElement>()
let timer: ReturnType<typeof setTimeout> | undefined
let observer: IntersectionObserver | undefined

const wait = (ms: number) =>
  new Promise<void>((resolve) => {
    timer = setTimeout(resolve, ms)
  })

function showAll() {
  shown.value = props.lines.map((l) => ({ kind: l.kind, text: l.text ?? '', typing: false }))
  done.value = true
}

async function play() {
  if (timer) clearTimeout(timer)
  shown.value = []
  done.value = false
  for (const line of props.lines) {
    const text = line.text ?? ''
    if (line.kind === 'cmd') {
      // Taken back out of the array so that Vue sees each character.
      shown.value.push({ kind: line.kind, text: '', typing: true })
      const entry = shown.value[shown.value.length - 1]
      await wait(250)
      for (const ch of text) {
        entry.text += ch
        await wait(ch === ' ' ? 70 : 28 + Math.random() * 30)
      }
      await wait(320)
      entry.typing = false
    } else {
      shown.value.push({ kind: line.kind, text, typing: false })
      await wait(line.kind === 'blank' ? 60 : 140)
    }
  }
  done.value = true
}

onMounted(() => {
  const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  if (still || !('IntersectionObserver' in window) || !root.value) {
    showAll()
    return
  }
  observer = new IntersectionObserver(
    (entries) => {
      if (entries.some((e) => e.isIntersecting)) {
        observer?.disconnect()
        play()
      }
    },
    { threshold: 0.4 },
  )
  observer.observe(root.value)
})

onBeforeUnmount(() => {
  if (timer) clearTimeout(timer)
  observer?.disconnect()
})
</script>

<template>
  <div ref="root" class="window">
    <div class="bar">
      <i /><i /><i />
      <span>{{ title ?? 'terminal' }}</span>
      <button v-if="done" class="replay" type="button" @click="play">Replay</button>
    </div>
    <pre class="term" aria-hidden="true"><span v-for="(l, i) in shown" :key="i" class="line" :class="l.kind"><template v-if="l.kind === 'cmd'"><span class="prompt">$</span> <b>{{ l.text }}</b><span v-if="l.typing" class="caret" /></template><template v-else>{{ l.text }}</template></span><span v-if="done" class="line cmd"><span class="prompt">$</span> <span class="caret" /></span></pre>
    <pre class="sr-only"><template v-for="(l, i) in lines" :key="i">{{ l.kind === 'cmd' ? '$ ' : '' }}{{ l.text ?? '' }}
</template></pre>
  </div>
</template>

<style scoped>
.window {
  width: 100%;
  border-radius: 12px;
  overflow: hidden;
  background: #121614;
  box-shadow: 0 24px 60px -24px rgba(10, 30, 24, 0.35);
  border: 1px solid #232a27;
}
.bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  background: #1a201d;
  font-size: 12px;
  color: #86928d;
}
.bar i { width: 11px; height: 11px; border-radius: 50%; background: #36403c; }
.bar span { margin-left: 8px; }
.replay {
  margin-left: auto;
  font-size: 11.5px;
  padding: 2px 10px;
  border: 1px solid #36403c;
  border-radius: 6px;
  color: #a7b3ad;
}
.replay:hover { color: #eef3f0; border-color: #4fd1a5; }
.term {
  margin: 0;
  padding: 18px 20px;
  min-height: 330px;
  overflow-x: auto;
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  line-height: 1.7;
  color: #c6d0cb;
}
.line { display: block; white-space: pre; min-height: 1.7em; }
.line.cmd b { color: #eef3f0; font-weight: 600; }
.line.ok { color: #7fd49a; }
.line.dim { color: #6b7772; }
.prompt { color: #4fd1a5; user-select: none; }
.caret {
  display: inline-block;
  width: 8px;
  height: 1.1em;
  margin-left: 2px;
  vertical-align: -0.2em;
  background: #7ddfbc;
  animation: blink 1s steps(1) infinite;
}
@keyframes blink { 50% { opacity: 0; } }
@media (prefers-reduced-motion: reduce) {
  .caret { animation: none; }
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  margin: -1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: pre;
}
</style>
