<script setup lang="ts">
import { onMounted, ref } from 'vue'

interface Row {
  what: string
  sbx: boolean
  note: string
  // What you would type without unibox, and what you type with it.
  before: string[]
  after: string[]
}

const rows: Row[] = [
  {
    what: 'A micro-VM per sandbox, with its own kernel and Docker',
    sbx: true,
    note: 'This is sbx\'s. unibox adds nothing here and takes nothing away.',
    before: ['sbx env create ...'],
    after: ['unibox run'],
  },
  {
    what: 'Tokens kept on the host, placeholders inside',
    sbx: true,
    note: 'Also sbx\'s: its proxy puts the token into requests to the hosts the env allows. unibox never reads one.',
    before: ['sbx secret set anthropic'],
    after: ['unibox secret set -e dev'],
  },
  {
    what: 'Set up from the team\'s repository in one command',
    sbx: false,
    note: 'Without unibox you clone the repository, find the right file and pass every arg yourself.',
    before: [
      'git clone git@github.com:acme/agent-envs.git',
      'cd agent-envs',
      'sbx env create --name dev --env-arg registryUser=tobola sandboxes/dev.sbxenv.yaml',
    ],
    after: ['unibox init git@github.com:acme/agent-envs.git'],
  },
  {
    what: 'Your values remembered',
    sbx: false,
    note: 'The env asks each person for a few values. sbx wants them on every create; unibox asks once.',
    before: ['sbx env create ... --env-arg registryUser=tobola --kit-arg toolchain:jdk=21'],
    after: ['unibox var set -e dev registryUser tobola    # once'],
  },
  {
    what: 'Which tokens are missing, at a glance',
    sbx: false,
    note: 'The kits say which services they sign in to. unibox reads them and compares with what sbx has.',
    before: ['grep -r service: kits/        # find out what is needed', 'sbx secret ls', 'sbx secret set anthropic', 'sbx secret set github'],
    after: ['unibox secret ls -e dev       # needed, present, missing', 'unibox secret set -e dev      # sbx asks for the missing ones'],
  },
  {
    what: 'Tasks you can resume, with any agent',
    sbx: false,
    note: 'A task is a repository in a sandbox with an agent in it. unibox keeps the list; you come back by name, and with whichever agent the sandbox has: start with claude, continue with codex.',
    before: [
      'sbx mount dev ~/code/api:/work/api',
      'sbx exec -it -w /work/api dev claude',
      '# later: remember what was mounted where',
    ],
    after: ['cd ~/code/api && unibox run --task login', 'unibox task ls', 'unibox resume login', 'unibox resume login --agent codex    # another agent, same task'],
  },
  {
    what: 'Clone instead of mount',
    sbx: false,
    note: 'When you would rather the agent never touched your checkout, it works on its own clone and you fetch its commits.',
    before: ['sbx exec -it dev bash', 'git clone https://github.com/acme/api /work/api', '# and keep the branch straight yourself'],
    after: ['unibox run --clone --branch feat/login'],
  },
  {
    what: 'Ports by name',
    sbx: false,
    note: 'The team names the ports everyone uses beside the env file. You switch them by name while the sandbox runs.',
    before: ['sbx ports dev --publish 5173:5173'],
    after: ['unibox port on vite'],
  },
  {
    what: 'Network rules per sandbox that survive a recreate',
    sbx: false,
    note: 'sbx keeps one policy, and a rule goes with the sandbox it was made for. unibox keeps yours and shows what was refused next to them.',
    before: ['sbx policy log --type network --limit 500   # what was refused', 'sbx policy allow network --sandbox dev jitpack.io', '# again, after every recreate'],
    after: ['unibox network ls -s dev', 'unibox network allow jitpack.io -s dev'],
  },
  {
    what: 'Your own CPUs and memory',
    sbx: false,
    note: 'The env file is the team\'s. Your resources are yours: numbers, or a share of the host, per env or sandbox.',
    before: ['# edit the team\'s sbxenv file, or keep a copy of your own'],
    after: ['unibox sandbox resources dev --cpus 50% --memory 16g'],
  },
  {
    what: 'Env changes applied, tasks kept',
    sbx: false,
    note: 'When the team changes the env, unibox says so at the next start. A recreate mounts and clones your tasks again.',
    before: ['git pull', 'sbx env rm dev --force', 'sbx env create ... sandboxes/dev.sbxenv.yaml', '# mount everything again'],
    after: ['unibox env update', 'unibox sandbox recreate dev'],
  },
  {
    what: 'A doctor that names the fix',
    sbx: false,
    note: 'sbx diagnose checks sbx. unibox doctor also checks the envs, your values, the tokens and the sandboxes, and prints the command for each problem.',
    before: ['sbx diagnose'],
    after: ['unibox doctor'],
  },
]

// With a mouse, resting on a line opens it; a click pins it so the next
// line hovered does not take its place, and a second click closes it.
const open = ref<number | null>(2)
const pinned = ref<number | null>(null)
const hovers = ref(false)
onMounted(() => {
  hovers.value = window.matchMedia('(hover: hover) and (pointer: fine)').matches
})

function toggle(i: number) {
  if (open.value === i && pinned.value === i) {
    open.value = null
    pinned.value = null
    return
  }
  open.value = i
  pinned.value = i
}
function enter(i: number) {
  if (hovers.value && pinned.value === null) open.value = i
}
</script>

<template>
  <div class="matrix" role="list">
    <div class="head" aria-hidden="true">
      <span />
      <span>sbx alone</span>
      <span class="us">with unibox</span>
    </div>
    <div
      v-for="(row, i) in rows"
      :key="row.what"
      class="row"
      :class="{ open: open === i }"
      role="listitem"
      @mouseenter="enter(i)"
    >
      <button
        type="button"
        class="line"
        :aria-expanded="open === i"
        :aria-controls="`cmp-${i}`"
        @click="toggle(i)"
      >
        <span class="what">{{ row.what }}</span>
        <span class="mark" :class="row.sbx ? 'yes' : 'no'" :aria-label="row.sbx ? 'sbx alone: yes' : 'sbx alone: by hand'">
          <svg v-if="row.sbx" viewBox="0 0 20 20"><path d="M4 10.5l4 4 8-9" /></svg>
          <svg v-else viewBox="0 0 20 20"><path d="M6 6l8 8M14 6l-8 8" /></svg>
        </span>
        <span class="mark yes us" aria-label="with unibox: yes">
          <svg viewBox="0 0 20 20"><path d="M4 10.5l4 4 8-9" /></svg>
        </span>
        <span class="chev" aria-hidden="true" />
      </button>
      <div :id="`cmp-${i}`" class="detail" :hidden="open !== i">
        <p class="note">{{ row.note }}</p>
        <div class="cols">
          <div class="col">
            <p class="label">sbx alone</p>
            <pre><span v-for="l in row.before" :key="l" class="l" :class="{ c: l.startsWith('#') }">{{ l }}</span></pre>
          </div>
          <div class="col us">
            <p class="label">with unibox</p>
            <pre><span v-for="l in row.after" :key="l" class="l" :class="{ c: l.startsWith('#') }">{{ l }}</span></pre>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.matrix {
  border: 1px solid var(--ub-frame);
  border-radius: 14px;
  overflow: hidden;
  background: var(--vp-c-bg);
}
.head,
.line {
  display: grid;
  grid-template-columns: 1fr 92px 108px 28px;
  align-items: center;
  gap: 8px;
  width: 100%;
  text-align: left;
}
@media (max-width: 560px) {
  .head,
  .line { grid-template-columns: 1fr 56px 64px 24px; }
}
.head {
  padding: 12px 20px;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--vp-c-text-3);
  background: var(--ub-code-bg);
  border-bottom: 1px solid var(--ub-frame);
}
.head span:not(:first-child) { text-align: center; }
.head .us { color: var(--ub-brand); font-weight: 600; }

.row + .row { border-top: 1px solid var(--ub-frame); }
.line {
  padding: 14px 20px;
  font-size: 15px;
  line-height: 1.5;
  color: var(--vp-c-text-1);
  cursor: pointer;
  transition: background-color 0.15s;
}
.line:hover,
.row.open .line { background: var(--ub-code-bg); }
.what { font-weight: 500; }
.mark {
  justify-self: center;
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border-radius: 50%;
}
.mark svg { width: 14px; height: 14px; fill: none; stroke: currentColor; stroke-width: 2.4; stroke-linecap: round; stroke-linejoin: round; }
.mark.yes { background: var(--ub-brand-soft); color: var(--ub-brand); }
.mark.no { background: rgba(140, 140, 150, 0.12); color: var(--vp-c-text-3); }
.chev {
  justify-self: center;
  width: 8px;
  height: 8px;
  border-right: 1.5px solid var(--vp-c-text-3);
  border-bottom: 1.5px solid var(--vp-c-text-3);
  transform: rotate(45deg);
  transition: transform 0.2s;
}
.row.open .chev { transform: rotate(-135deg); }

.detail {
  padding: 4px 20px 22px;
  background: var(--ub-code-bg);
}
.note {
  margin: 0 0 14px;
  max-width: 760px;
  font-size: 14.5px;
  line-height: 1.65;
  color: var(--vp-c-text-2);
}
.cols { display: grid; gap: 12px; }
@media (min-width: 760px) { .cols { grid-template-columns: 1fr 1fr; gap: 16px; } }
.col {
  border: 1px solid var(--ub-frame);
  border-radius: 10px;
  background: var(--vp-c-bg);
  overflow: hidden;
}
.col.us { border-color: var(--ub-brand); }
.label {
  margin: 0;
  padding: 7px 14px;
  font-size: 11.5px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--vp-c-text-3);
  border-bottom: 1px solid var(--ub-frame);
}
.col.us .label { color: var(--ub-brand); border-bottom-color: var(--ub-brand-soft); }
pre {
  margin: 0;
  padding: 12px 14px;
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  line-height: 1.75;
  color: var(--vp-c-text-1);
  overflow-x: auto;
}
.l { display: block; white-space: pre; }
.l.c { color: var(--vp-c-text-3); }
</style>
