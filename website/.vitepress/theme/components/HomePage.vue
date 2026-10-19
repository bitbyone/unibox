<script setup lang="ts">
import { ref } from 'vue'
import { withBase } from 'vitepress'

const installs = {
  Homebrew: 'brew tap bitbyone/unibox https://github.com/bitbyone/unibox\nbrew trust bitbyone/unibox\nbrew install unibox',
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

const flow = [
  {
    where: 'Environments repository',
    what: 'git, yours',
    lines: ['sandboxes/<profile>/', '  sandbox.yaml', '  sandbox.dockerfile', '_shared/  .shared/'],
  },
  {
    where: 'Host',
    what: 'unibox and the runtime',
    lines: ['unibox: profile → settings', 'sbx: the VM,', '  credential proxy,', '  network policy'],
  },
  {
    where: 'Sandbox',
    what: 'a micro-VM',
    lines: ['/work/<task>/<repo>', 'a fresh clone, its branch', 'claude · codex · copilot', 'commits and pushes'],
  },
  {
    where: 'Forge',
    what: 'GitHub, GitLab',
    lines: ['the branch arrives', 'protected branches', 'you pull, as from', 'a colleague'],
  },
]

const features = [
  {
    title: 'One profile format',
    text: 'A sandbox.yaml and a Dockerfile in a git repository of yours. It names no runtime: the same profile runs in the experimental msb too.',
    link: '/guide/profiles',
  },
  {
    title: 'Setup done by commands',
    text: 'unibox init lays out the repository, unibox agent login and unibox service add sign agents and forges in. No runtime configuration by hand.',
    link: '/guide/',
  },
  {
    title: 'Logins kept on the host',
    text: 'Tokens stay in the keychain. unibox hands them to the runtime, whose proxy puts them into requests; the sandbox sees placeholders.',
    link: '/guide/services-and-secrets',
  },
  {
    title: 'Agents side by side',
    text: 'claude, codex, copilot and opencode in one image, each signed in and credited in commits.',
    link: '/guide/agents',
  },
  {
    title: 'Tasks on clones',
    text: 'A task is clones of your repositories on a branch inside a sandbox. Resume it later, with any agent of the profile.',
    link: '/guide/tasks',
  },
  {
    title: 'Network rules and ports by name',
    text: 'Allow or deny hosts and publish ports with a command; sbx applies a change to running sandboxes.',
    link: '/guide/network-and-ports',
  },
  {
    title: 'Images rebuilt when needed',
    text: 'An image is tagged with the profile\'s revision, built again when an input changed, and old ones are removed.',
    link: '/guide/images',
  },
  {
    title: 'Scriptable',
    text: 'Every command takes --json and --yes, with a versioned JSON contract, so a program such as unagit can drive it.',
    link: '/reference/cli',
  },
]
</script>

<template>
  <div class="home">
    <section class="hero">
      <p class="kicker">A wrapper for Docker Sandboxes</p>
      <h1>Coding agents in local sandboxes,<br /><span>from one profile format.</span></h1>
      <p class="lead">
        unibox is a command-line wrapper over Docker Sandboxes (sbx). It gives it a profile format
        and sets up the rest - the image, the agents' logins, forge tokens, network rules - so an
        agent runs in a sandbox on your machine with one command. The isolation, the credential
        proxy and the network policy are sbx's; unibox configures them. An experimental adapter for
        microsandbox (msb) shows the same profile runs elsewhere. It is for local development, and
        nothing more.
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
        <pre><code><span v-for="line in installs[way].split('\n')" :key="line" class="line"><span class="prompt">$</span> {{ line }}</span></code></pre>
      </div>

      <div class="actions">
        <a class="btn brand" :href="withBase('/guide/')">Get Started</a>
        <a class="btn alt" :href="withBase('/guide/why')">What it is</a>
        <a class="btn alt" href="https://github.com/bitbyone/unibox">GitHub</a>
      </div>
    </section>

    <section class="how" aria-label="How unibox works">
      <h2>How it works</h2>
      <p class="sub">
        A profile describes the environment. unibox turns it into the runtime's settings. The agent
        works on clones in the sandbox and pushes to a branch, which you pull.
      </p>
      <ol class="flow">
        <li v-for="(step, i) in flow" :key="step.where" class="step">
          <div class="step-head">
            <span class="num">{{ i + 1 }}</span>
            <div>
              <h3>{{ step.where }}</h3>
              <p>{{ step.what }}</p>
            </div>
          </div>
          <pre><span v-for="l in step.lines" :key="l" class="line">{{ l }}</span></pre>
        </li>
      </ol>
      <p class="note">
        The runtime's proxy puts the real token into requests to the hosts a login or a service
        names. Inside the VM there is only a placeholder.
      </p>
    </section>

    <section class="screen" aria-label="unibox in a terminal">
      <div class="window">
        <div class="bar"><i /><i /><i /><span>~/src/web</span></div>
        <pre class="term"><span class="dim"># create sandbox web-1 from profile web, clone this repository, run claude</span>
<span class="dim">$</span> <b>unibox run --profile web --sandbox web-1 --branch feat/login --agent claude</b>

<span class="dim"># every task in every sandbox, and one picked up again later</span>
<span class="dim">$</span> <b>unibox tasks</b>
<span class="dim">$</span> <b>unibox resume</b> <em>&lt;task&gt;</em> <b>--agent codex</b>

<span class="dim"># reach a host the policy refuses: written into the profile</span>
<span class="dim">$</span> <b>unibox network allow web jitpack.io</b>

<span class="dim"># publish the dev server on localhost by name, at every start</span>
<span class="dim">$</span> <b>unibox network port web fe 3000:3000</b></pre>
      </div>
    </section>

    <section class="arrangement">
      <div class="copy-col">
        <h2>Work comes back through the forge</h2>
        <p>
          No host directory is mounted by default, and nothing of the sandbox reaches your
          checkout. A <code>.git</code> the agent could write holds commands your machine would
          run later: hooks, <code>core.fsmonitor</code>, <code>credential.helper</code>. A fetch
          from the forge carries objects and refs, never config or hooks.
        </p>
        <a :href="withBase('/reference/security')">What crosses the boundary →</a>
      </div>
      <pre class="formula"><span>host</span>      your checkout   <span class="dim">← git pull ─┐</span>
                                       <span class="dim">│</span>
<span>forge</span>     the task's branch <span class="dim">◄──────┤</span>
                                       <span class="dim">│</span>
<span>sandbox</span>   /work/&lt;task&gt;/&lt;repo&gt; <span class="dim">─ git push</span></pre>
    </section>

    <section class="features">
      <h2>What unibox does</h2>
      <div class="grid">
        <a v-for="f in features" :key="f.title" class="card" :href="withBase(f.link)">
          <h3>{{ f.title }}</h3>
          <p>{{ f.text }}</p>
        </a>
      </div>
    </section>

    <section class="closing">
      <h2>Getting started</h2>
      <p>
        <code>unibox init --dir ~/my-envs</code> lays out an environments repository,
        <code>unibox init web</code> adds a profile, <code>unibox agent login web</code> signs its
        agents in, and <code>unibox run --profile web</code> starts one.
      </p>
      <a class="btn brand" :href="withBase('/guide/')">Getting Started</a>
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
  max-width: 700px;
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

h2 {
  font-size: clamp(26px, 3.4vw, 36px);
  line-height: 1.2;
  letter-spacing: -0.025em;
  font-weight: 600;
  color: var(--vp-c-text-1);
}

.how { text-align: center; }
.how .sub {
  max-width: 680px;
  margin: 16px auto 48px;
  font-size: 17px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
.flow {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 16px;
  text-align: left;
}
@media (min-width: 720px) { .flow { grid-template-columns: repeat(2, 1fr); } }
@media (min-width: 1100px) { .flow { grid-template-columns: repeat(4, 1fr); gap: 28px; } }
.step {
  position: relative;
  border: 1px solid var(--ub-frame);
  border-radius: 14px;
  padding: 20px;
  background: var(--vp-c-bg);
}
@media (min-width: 1100px) {
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
.step-head { display: flex; gap: 12px; align-items: flex-start; }
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
.step h3 { font-size: 16px; font-weight: 600; color: var(--vp-c-text-1); line-height: 1.4; }
.step-head p { font-size: 13px; color: var(--vp-c-text-3); }
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
.how .note {
  max-width: 640px;
  margin: 36px auto 0;
  font-size: 15px;
  color: var(--vp-c-text-2);
}

.screen { display: flex; justify-content: center; background: var(--ub-code-bg); }
.window {
  width: 100%;
  max-width: 860px;
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
.term {
  margin: 0;
  padding: 18px 20px;
  overflow-x: auto;
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  line-height: 1.65;
  color: #c6d0cb;
}
.term b { color: #eef3f0; font-weight: 600; }
.term em { font-style: normal; color: #7ddfbc; }
.term .dim { color: #6b7772; }
.term .ok { color: #7fd49a; }
.term .run { color: #e0a15c; }

.arrangement {
  display: grid;
  gap: 40px;
  align-items: center;
}
@media (min-width: 960px) {
  .arrangement { grid-template-columns: 1fr 1fr; gap: 64px; }
}
.arrangement p { margin: 16px 0; font-size: 17px; line-height: 1.75; color: var(--vp-c-text-2); }
.arrangement code,
.closing code {
  font-size: 0.88em;
  padding: 2px 6px;
  border-radius: 5px;
  background: var(--ub-brand-soft);
  color: var(--ub-brand);
}
.arrangement a { color: var(--ub-brand); font-weight: 500; }
.formula {
  margin: 0;
  padding: 28px;
  border-radius: 12px;
  background: var(--ub-code-bg);
  border: 1px solid var(--ub-frame);
  font-family: var(--vp-font-family-mono);
  font-size: 14px;
  line-height: 1.9;
  overflow-x: auto;
  color: var(--vp-c-text-1);
}
.formula span { color: var(--ub-brand); }
.formula .dim { color: var(--vp-c-text-3); }

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

.closing { text-align: center; border-bottom: none; }
.closing p { margin: 16px auto 28px; max-width: 560px; font-size: 17px; color: var(--vp-c-text-2); }
</style>
