<script setup lang="ts">
import { ref } from 'vue'
import { withBase } from 'vitepress'
import ArchDiagram from './ArchDiagram.vue'
import Terminal, { type TermLine } from './Terminal.vue'
import Compare from './Compare.vue'

const installs = {
  Homebrew: 'brew tap bitbyone/unibox https://github.com/bitbyone/unibox\nbrew trust bitbyone/unibox\nbrew install unibox',
  'Scoop (Windows)': 'scoop bucket add unibox https://github.com/bitbyone/unibox\nscoop install unibox',
  'From source': 'git clone https://github.com/bitbyone/unibox\ncd unibox && make install',
}
type Way = keyof typeof installs
const way = ref<Way>('Homebrew')
const copied = ref(false)

async function copy() {
  try {
    await navigator.clipboard.writeText(installs[way.value])
    copied.value = true
    setTimeout(() => (copied.value = false), 1600)
  } catch {
    // A browser that refuses the clipboard leaves the command to be selected by hand.
  }
}

const reasons = [
  {
    title: 'A machine of its own',
    text: 'The sandbox is a micro-VM with its own kernel, disk and Docker. The agent has root there and nothing anywhere else: it sees the repositories you give it, at /work/<task>/<repo>, not your home directory.',
  },
  {
    title: 'Tokens it never holds',
    text: 'Your GitHub, Anthropic or registry tokens stay in sbx\'s store on your machine. The agent gets placeholders. sbx\'s proxy puts the real token into each request, and only for the hosts the env allows.',
  },
  {
    title: 'A network with a list',
    text: 'The sandbox reaches only the hosts the env names. unibox shows you what was refused and lets you allow a host for one sandbox. The ports you choose appear on localhost, nowhere else.',
  },
]

const steps = [
  {
    who: 'Your team',
    when: 'once, in git',
    title: 'Describes the sandbox',
    text: 'A repository of sbxenv files and kits, in Docker Sandboxes\' own format: which image, which tools, which agent, which hosts it may reach and which tokens it needs. Nothing in it is specific to unibox.',
    code: ['sandboxes/dev.sbxenv.yaml', 'kits/agent/', 'kits/toolchain/'],
  },
  {
    who: 'You',
    when: 'once',
    title: 'Set it up',
    text: 'unibox init clones that repository, asks for the values and tokens that are yours alone, and has sbx create the sandbox. Five steps; each one checks itself and can be run again.',
    code: ['$ unibox init git@github.com:acme/agent-envs.git'],
  },
  {
    who: 'You',
    when: 'every day',
    title: 'Run the agent',
    text: 'unibox run in a repository mounts it into the sandbox and starts the agent in your terminal. The work is a task: it stays, and unibox resume picks it up later, with the same agent or any other the sandbox has.',
    code: ['$ cd ~/code/api', '$ unibox run'],
  },
]

const session: TermLine[] = [
  { kind: 'cmd', text: 'unibox init git@github.com:acme/agent-envs.git' },
  { kind: 'dim', text: 'cloned github.com/acme/agent-envs into ~/.local/share/unibox/repos' },
  { kind: 'out', text: '1/5 sbx         installed and signed in' },
  { kind: 'out', text: '2/5 Envs        env dev, sandboxes/dev.sbxenv.yaml: added' },
  { kind: 'out', text: '3/5 Values      registryUser? tobola' },
  { kind: 'out', text: '4/5 Secrets     sbx has no token for anthropic of env dev' },
  { kind: 'dim', text: '                sbx asks for each on this terminal, and the value goes to it alone' },
  { kind: 'out', text: '5/5 Sandboxes   sandbox dev created' },
  { kind: 'ok', text: 'set up; start a task with unibox run <directory>' },
  { kind: 'blank' },
  { kind: 'cmd', text: 'cd ~/code/api && unibox run --task login --branch feat/login' },
  { kind: 'out', text: 'mounted ~/code/api at /work/login/api in sandbox dev' },
  { kind: 'dim', text: 'starting claude in /work/login ...' },
  { kind: 'blank' },
  { kind: 'cmd', text: 'unibox port on vite' },
  { kind: 'ok', text: 'vite  http://127.0.0.1:5173  on' },
  { kind: 'blank' },
  { kind: 'cmd', text: 'unibox resume login --agent codex' },
  { kind: 'dim', text: 'starting codex in /work/login, where claude left off ...' },
]

const features = [
  {
    title: 'One command to set up',
    text: 'unibox init clones the env repository, asks for your values and tokens, and creates the sandbox. Run it again any time: it adds what is new and skips what is done.',
    link: '/guide/',
  },
  {
    title: 'Tokens stay on your machine',
    text: 'sbx stores them; its proxy adds them to requests. The sandbox sees only placeholders, and unibox itself never reads a value.',
    link: '/guide/secrets',
  },
  {
    title: 'Resume a task with any agent',
    text: 'Start with claude, continue with codex. unibox resume picks the task up where it was, with the same agent or any other the sandbox has.',
    link: '/guide/tasks',
  },
  {
    title: 'Mount or clone',
    text: 'Work on your checkout and see each change at once, or on a clone from GitHub or GitLab that never touches your .git.',
    link: '/guide/tasks#mount-or-clone',
  },
  {
    title: 'Ports by name',
    text: 'The team names the ports beside the env. unibox port on vite publishes the dev server on localhost.',
    link: '/guide/ports',
  },
  {
    title: 'Network rules per sandbox',
    text: 'See which hosts were refused and allow one with unibox network allow. The rules survive a recreate.',
    link: '/guide/network',
  },
  {
    title: 'Env changes, on your terms',
    text: 'When the team changes the env, unibox says so at the next start. sandbox recreate applies it and restores your tasks.',
    link: '/guide/updating',
  },
  {
    title: 'A doctor that names the fix',
    text: 'unibox doctor checks sbx, the envs, your values, the tokens and the sandboxes, and prints the unibox command for each problem.',
    link: '/guide/troubleshooting',
  },
  {
    title: 'Scriptable',
    text: 'Every command takes --json and --yes. Linux, macOS and Windows, one binary, no dependencies.',
    link: '/reference/cli',
  },
]
</script>

<template>
  <div class="home">
    <section class="hero">
      <p class="kicker">Open source · a CLI for Docker Sandboxes</p>
      <h1>Let coding agents work in a sandbox,<br /><span>not on your laptop.</span></h1>
      <p class="lead">
        unibox runs Claude Code, Codex, Copilot or OpenCode inside a Docker Sandbox: a small VM on
        your machine that gets your repository and nothing else. Your team describes the sandbox
        once, in git. You install unibox and type one command.
      </p>

      <div class="install">
        <div class="install-tabs" role="tablist">
          <button
            v-for="(_, name) in installs"
            :key="name"
            role="tab"
            :class="{ on: way === name }"
            :aria-selected="way === name"
            @click="way = name as Way"
          >{{ name }}</button>
          <button class="copy" :aria-label="copied ? 'Copied' : 'Copy the commands'" @click="copy">
            {{ copied ? 'Copied' : 'Copy' }}
          </button>
        </div>
        <pre><code><span v-for="line in installs[way].split('\n')" :key="line" class="line"><span class="prompt">{{ way.startsWith('Scoop') ? 'PS>' : '$' }}</span> {{ line }}</span></code></pre>
      </div>

      <div class="actions">
        <a class="btn brand" :href="withBase('/guide/')">Get started</a>
        <a class="btn alt" href="#how">How it works</a>
        <a class="btn alt" href="https://github.com/bitbyone/unibox">GitHub</a>
      </div>
      <p class="needs">
        Needs <a href="https://docs.docker.com/ai/sandboxes/">Docker Sandboxes</a> 0.49.2 or newer
        and git · Linux, macOS and Windows · one binary · MIT
      </p>
    </section>

    <section class="audience" aria-label="Where to start">
      <div class="aud">
        <p class="aud-kicker">New to Docker Sandboxes?</p>
        <h2>You will not have to learn them.</h2>
        <p>
          Docker Sandboxes (<code>sbx</code>) is Docker's way of running an agent in a VM of its
          own, with a proxy that keeps your tokens out of it. unibox sets it up for you from your
          team's repository and gives you the few commands you need. You never type an
          <code>sbx</code> command.
        </p>
        <a href="#why">Why a sandbox at all →</a>
      </div>
      <div class="aud">
        <p class="aud-kicker">Already using sbx?</p>
        <h2>Keep your files. Lose the chores.</h2>
        <p>
          unibox takes your sbxenv files and kits exactly as they are and adds what sbx leaves to
          each person: their values for the env's args, a check of which tokens are missing, tasks
          you can resume, ports by name, and a doctor. Remove unibox and the files still work with
          <code>sbx env create</code>.
        </p>
        <a href="#compare">What unibox adds to sbx →</a>
      </div>
    </section>

    <section id="why" class="why">
      <h2>An agent on your machine has your machine.</h2>
      <p class="sub">
        A coding agent runs whatever commands it decides to. Run it directly and it has your SSH
        keys, every token in your shell and every file in your home. A sandbox gives it a machine
        of its own, and gives you a way to say what it may reach.
      </p>
      <div class="reasons">
        <div v-for="(r, i) in reasons" :key="r.title" class="reason">
          <span class="num">{{ i + 1 }}</span>
          <h3>{{ r.title }}</h3>
          <p>{{ r.text }}</p>
        </div>
      </div>
    </section>

    <section class="picture" aria-label="What unibox run does">
      <h2>What <code>unibox run</code> does</h2>
      <p class="sub">
        Your repository goes in. Requests come out through the proxy on your machine, where the
        token is added. The token itself never goes in.
      </p>
      <ArchDiagram />
    </section>

    <section id="how" class="how" aria-label="How unibox works">
      <h2>How it works</h2>
      <p class="sub">Three steps. Only the last one is daily work.</p>
      <ol class="flow">
        <li v-for="(step, i) in steps" :key="step.title" class="step">
          <div class="step-head">
            <span class="num">{{ i + 1 }}</span>
            <p class="who">{{ step.who }} <span>· {{ step.when }}</span></p>
          </div>
          <h3>{{ step.title }}</h3>
          <p class="text">{{ step.text }}</p>
          <pre><span v-for="l in step.code" :key="l" class="line">{{ l }}</span></pre>
        </li>
      </ol>
    </section>

    <section class="screen" aria-label="unibox in a terminal">
      <div class="screen-inner">
        <div class="screen-copy">
          <h2>The whole thing, in one terminal</h2>
          <p>
            Set up once, then a task per piece of work. Each command says what it did; what it could
            not do comes with the command that fixes it.
          </p>
          <ul>
            <li><code>unibox init</code> walks five steps and can be run again</li>
            <li><code>unibox run</code> mounts the repository and starts the agent</li>
            <li><code>unibox port on</code> opens a port on localhost</li>
            <li><code>unibox resume</code> picks a task up later, with any agent</li>
          </ul>
          <a :href="withBase('/guide/')">Getting started →</a>
        </div>
        <Terminal :lines="session" title="~/code/api" />
      </div>
    </section>

    <section class="arrangement">
      <div class="copy-col">
        <h2>Mount or clone: you choose how close the agent gets</h2>
        <p>
          By default a task <strong>mounts</strong> your checkout: the agent edits your files, and
          you see every change in your editor at once. The agent can also write into your
          <code>.git</code>, such as a hook your machine will run later. Mount repositories you
          would let it configure.
        </p>
        <p>
          With <code>--clone</code> the agent works on its own clone from GitHub or GitLab, on a
          branch of the task's. Your checkout is never touched; you fetch its commits when you want
          them.
        </p>
        <a :href="withBase('/guide/tasks#mount-or-clone')">Mount or clone, in detail →</a>
      </div>
      <div class="modes">
        <div class="mode">
          <div class="mode-head"><span class="tag">default</span><h3>Mount</h3></div>
          <div class="mode-row">
            <span class="node">your checkout</span>
            <span class="link both">live, both ways</span>
            <span class="node vm">/work/login/api</span>
          </div>
          <p>Changes appear on your disk as they are made. Your <code>.git</code> is in reach.</p>
        </div>
        <div class="mode">
          <div class="mode-head"><span class="tag alt">--clone</span><h3>Clone</h3></div>
          <div class="mode-row">
            <span class="node">your checkout</span>
            <span class="link fetch">git fetch</span>
            <span class="node">GitHub / GitLab</span>
            <span class="link push">git push</span>
            <span class="node vm">/work/login/api</span>
          </div>
          <p>The agent has its own clone. Your checkout stays yours.</p>
        </div>
      </div>
    </section>

    <section id="compare" class="compare" aria-label="unibox beside sbx">
      <h2>What unibox adds to sbx</h2>
      <p class="sub">
        unibox is a layer, not a replacement: the isolation, the proxy and the network policy
        are sbx's, and the env files stay plain sbx. What unibox adds is the part you would
        otherwise do by hand.
      </p>
      <Compare />
      <p class="hint">Click a line for what it saves you, as the commands you would type.</p>
    </section>

    <section class="features">
      <h2>What you get</h2>
      <div class="grid">
        <a v-for="f in features" :key="f.title" class="card" :href="withBase(f.link)">
          <h3>{{ f.title }}</h3>
          <p>{{ f.text }}</p>
        </a>
      </div>
    </section>

    <section class="closing">
      <h2>Start in ten minutes</h2>
      <p>
        Install unibox, run <code>unibox init</code> with your team's env repository, then
        <code>unibox run</code> in a repository. If your team has no env repository yet,
        <a :href="withBase('/guide/env-authors')">For Env Authors</a> says how to write one.
      </p>
      <div class="actions">
        <a class="btn brand" :href="withBase('/guide/')">Getting started</a>
        <a class="btn alt" :href="withBase('/guide/how-it-works')">How it works, in detail</a>
      </div>
    </section>
  </div>
</template>

<style scoped>
.home {
  max-width: 1440px;
  margin: 0 auto;
}
section {
  padding: 72px 24px;
  border-bottom: 1px solid var(--ub-frame);
}
@media (min-width: 768px) {
  section { padding: 96px 64px; }
}
h2 {
  font-size: clamp(26px, 3.4vw, 36px);
  line-height: 1.2;
  letter-spacing: -0.025em;
  font-weight: 600;
  color: var(--vp-c-text-1);
}
h2 code,
.sub code,
.aud code,
.arrangement code,
.screen-copy code,
.closing code,
.modes code {
  font-size: 0.86em;
  padding: 2px 6px;
  border-radius: 5px;
  background: var(--ub-brand-soft);
  color: var(--ub-brand);
}
.sub {
  max-width: 680px;
  margin: 16px auto 48px;
  font-size: 17px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
.num {
  flex: none;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  font-size: 13px;
  font-weight: 600;
  background: var(--ub-brand-soft);
  color: var(--ub-brand);
}

/* hero */
.hero {
  text-align: center;
  padding-top: 96px;
}
.kicker {
  display: inline-block;
  font-size: 13px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--vp-c-text-2);
  border: 1px solid var(--ub-frame);
  border-radius: 999px;
  padding: 4px 14px;
  margin-bottom: 28px;
}
.hero h1 {
  font-size: clamp(36px, 6vw, 64px);
  line-height: 1.08;
  letter-spacing: -0.035em;
  font-weight: 600;
  color: var(--vp-c-text-1);
}
.hero h1 span { color: var(--ub-brand); }
.lead {
  max-width: 720px;
  margin: 24px auto 0;
  font-size: 18px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
.install {
  max-width: 600px;
  margin: 40px auto 0;
  text-align: left;
  border: 1px solid var(--ub-frame);
  border-radius: 12px;
  overflow: hidden;
  background: var(--ub-code-bg);
}
.install-tabs {
  display: flex;
  gap: 4px;
  padding: 6px;
  border-bottom: 1px solid var(--ub-frame);
  background: var(--vp-c-bg);
}
.install-tabs button {
  font-size: 13px;
  padding: 4px 12px;
  border-radius: 6px;
  color: var(--vp-c-text-2);
}
.install-tabs button.on {
  background: var(--ub-brand-soft);
  color: var(--ub-brand);
}
.install pre {
  margin: 0;
  padding: 18px 20px;
  overflow-x: auto;
  font-family: var(--vp-font-family-mono);
  font-size: 14px;
  line-height: 1.9;
}
.install .line { display: block; white-space: pre; }
.install .prompt { color: var(--vp-c-text-3); user-select: none; }
.copy {
  margin-left: auto;
  font-size: 12px;
  padding: 4px 10px;
  border: 1px solid var(--ub-frame);
  border-radius: 6px;
  background: var(--vp-c-bg);
  color: var(--vp-c-text-2);
}
.copy:hover { color: var(--ub-brand); }
.actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 12px;
  margin-top: 32px;
}
.btn {
  display: inline-block;
  padding: 10px 22px;
  border-radius: 999px;
  font-size: 15px;
  font-weight: 500;
  border: 1px solid transparent;
  transition: background-color 0.2s, border-color 0.2s;
}
.btn.brand { background: var(--ub-brand); color: #fff; }
.dark .btn.brand { color: #0c0c10; }
.btn.brand:hover { background: var(--ub-brand-hover); }
.btn.alt { border-color: var(--ub-frame); color: var(--vp-c-text-1); }
.btn.alt:hover { border-color: var(--vp-c-text-3); }
.needs {
  margin-top: 28px;
  font-size: 13.5px;
  color: var(--vp-c-text-3);
}
.needs a { color: var(--vp-c-text-2); text-decoration: underline; text-underline-offset: 3px; }

/* two audiences */
.audience {
  display: grid;
  gap: 24px;
}
@media (min-width: 900px) {
  .audience { grid-template-columns: 1fr 1fr; gap: 32px; }
}
.aud {
  border: 1px solid var(--ub-frame);
  border-radius: 16px;
  padding: 32px;
  background: var(--vp-c-bg);
}
.aud-kicker {
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--ub-brand);
}
.aud h2 { margin-top: 10px; font-size: clamp(22px, 2.4vw, 28px); }
.aud p:not(.aud-kicker) { margin-top: 14px; font-size: 16px; line-height: 1.75; color: var(--vp-c-text-2); }
.aud a,
.arrangement a,
.screen-copy a { display: inline-block; margin-top: 18px; color: var(--ub-brand); font-weight: 500; }

/* why a sandbox */
.why { text-align: center; }
.reasons {
  display: grid;
  gap: 16px;
  text-align: left;
}
@media (min-width: 900px) { .reasons { grid-template-columns: repeat(3, 1fr); gap: 24px; } }
.reason {
  border: 1px solid var(--ub-frame);
  border-radius: 14px;
  padding: 24px;
  background: var(--ub-code-bg);
}
.reason h3 { margin-top: 16px; font-size: 18px; font-weight: 600; color: var(--vp-c-text-1); }
.reason p { margin-top: 8px; font-size: 15px; line-height: 1.65; color: var(--vp-c-text-2); }

/* the picture */
.picture { text-align: center; }
.picture .sub { margin-bottom: 40px; }
.picture figure { max-width: 980px; margin: 0 auto; }

/* how it works */
.how { text-align: center; }
.flow {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 16px;
  text-align: left;
}
@media (min-width: 900px) { .flow { grid-template-columns: repeat(3, 1fr); gap: 28px; } }
.step {
  position: relative;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--ub-frame);
  border-radius: 14px;
  padding: 20px;
  background: var(--vp-c-bg);
}
@media (min-width: 900px) {
  .step:not(:last-child)::after {
    content: '→';
    position: absolute;
    right: -22px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--ub-accent);
    font-size: 18px;
  }
}
.step-head { display: flex; gap: 10px; align-items: center; }
.who { font-size: 13px; font-weight: 600; color: var(--ub-brand); }
.who span { font-weight: 400; color: var(--vp-c-text-3); }
.step h3 { margin-top: 14px; font-size: 18px; font-weight: 600; color: var(--vp-c-text-1); line-height: 1.4; }
.step .text { margin-top: 6px; font-size: 15px; line-height: 1.6; color: var(--vp-c-text-2); flex: 1; }
.step pre {
  margin: 16px 0 0;
  padding: 12px 14px;
  border-radius: 8px;
  background: var(--ub-code-bg);
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  overflow-x: auto;
}
.step pre .line { display: block; white-space: pre; }

/* the terminal */
.screen { background: var(--ub-code-bg); }
.screen-inner {
  display: grid;
  gap: 36px;
  align-items: center;
  max-width: 1200px;
  margin: 0 auto;
}
@media (min-width: 1024px) {
  .screen-inner { grid-template-columns: 2fr 3fr; gap: 56px; }
}
.screen-copy p { margin-top: 16px; font-size: 17px; line-height: 1.75; color: var(--vp-c-text-2); }
.screen-copy ul { margin: 20px 0 0; padding: 0; list-style: none; }
.screen-copy li {
  position: relative;
  padding-left: 22px;
  margin-top: 8px;
  font-size: 15px;
  line-height: 1.6;
  color: var(--vp-c-text-2);
}
.screen-copy li::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0.65em;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--ub-brand-soft);
  border: 1.5px solid var(--ub-brand);
}

/* mount or clone */
.arrangement {
  display: grid;
  gap: 40px;
  align-items: center;
}
@media (min-width: 960px) {
  .arrangement { grid-template-columns: 5fr 6fr; gap: 64px; }
}
.arrangement p { margin: 16px 0 0; font-size: 17px; line-height: 1.75; color: var(--vp-c-text-2); }
.arrangement strong { color: var(--vp-c-text-1); font-weight: 600; }
.modes { display: grid; gap: 16px; }
.mode {
  border: 1px solid var(--ub-frame);
  border-radius: 14px;
  padding: 20px 22px;
  background: var(--ub-code-bg);
}
.mode-head { display: flex; align-items: center; gap: 10px; }
.mode-head h3 { font-size: 17px; font-weight: 600; color: var(--vp-c-text-1); }
.tag {
  font-family: var(--vp-font-family-mono);
  font-size: 11.5px;
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--ub-brand-soft);
  color: var(--ub-brand);
}
.tag.alt { background: rgba(194, 122, 44, 0.12); color: var(--ub-accent); }
.mode-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 16px;
}
.node {
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  line-height: 1.5;
  padding: 6px 10px;
  border-radius: 8px;
  border: 1px solid var(--ub-frame);
  background: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  white-space: nowrap;
}
.node.vm { border-color: var(--ub-brand); border-style: dashed; }
.link {
  position: relative;
  flex: 1 1 60px;
  min-width: 60px;
  height: 31px;
  display: flex;
  justify-content: center;
  align-items: flex-start;
  font-size: 11px;
  line-height: 1;
  color: var(--vp-c-text-3);
  background: linear-gradient(currentColor, currentColor) center / 100% 1.5px no-repeat;
}
.link.both { color: var(--ub-brand); }
.link::before,
.link::after {
  content: '';
  position: absolute;
  top: 50%;
  width: 7px;
  height: 7px;
  margin-top: -4px;
  border-top: 1.5px solid currentColor;
  border-right: 1.5px solid currentColor;
}
.link::after { right: 0; transform: rotate(45deg); }
.link::before { left: 0; transform: rotate(-135deg); }
.link.fetch::after,
.link.push::after { display: none; }
.mode p { margin-top: 14px; font-size: 14px; line-height: 1.6; color: var(--vp-c-text-2); }

/* beside sbx */
.compare h2 { text-align: center; }
.compare .sub { text-align: center; margin-bottom: 40px; }
.compare .hint {
  margin: 14px 0 0;
  text-align: center;
  font-size: 13px;
  color: var(--vp-c-text-3);
}

/* features */
.features h2 { text-align: center; margin-bottom: 48px; }
.grid {
  display: grid;
  grid-template-columns: 1fr;
  border: 1px solid var(--ub-frame);
  border-radius: 14px;
  overflow: hidden;
}
@media (min-width: 640px) { .grid { grid-template-columns: repeat(2, 1fr); } }
@media (min-width: 1024px) { .grid { grid-template-columns: repeat(3, 1fr); } }
.card {
  padding: 28px;
  margin: 0 -1px -1px 0;
  border-right: 1px solid var(--ub-frame);
  border-bottom: 1px solid var(--ub-frame);
  transition: background-color 0.2s;
}
.card:hover { background: var(--ub-code-bg); }
.card h3 { font-size: 17px; font-weight: 600; color: var(--vp-c-text-1); }
.card p { margin-top: 8px; font-size: 15px; line-height: 1.65; color: var(--vp-c-text-2); }

/* closing */
.closing { text-align: center; border-bottom: none; }
.closing p { margin: 16px auto 0; max-width: 600px; font-size: 17px; line-height: 1.7; color: var(--vp-c-text-2); }
.closing p a { color: var(--ub-brand); text-decoration: underline; text-underline-offset: 3px; }
</style>
