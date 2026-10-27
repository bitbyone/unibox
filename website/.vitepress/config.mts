import { defineConfig } from 'vitepress'

// The site is served by GitHub Pages from the repository's own address,
// https://bitbyone.github.io/unibox/, so every link is under /unibox/.
const base = '/unibox/'
const repo = 'https://github.com/bitbyone/unibox'

const guideSidebar = [
  {
    text: 'Getting Started',
    items: [
      { text: 'Introduction', link: '/' },
      { text: 'Installation', link: '/guide/installation' },
      { text: 'Usage', link: '/guide/usage' },
    ],
  },
  {
    text: 'Daily Work',
    items: [
      { text: 'Setting Up: init', link: '/guide/init' },
      { text: 'Tasks', link: '/guide/tasks' },
      { text: 'Interactive Lists', link: '/guide/interactive' },
      { text: 'Repository Index', link: '/guide/repos' },
      { text: 'Agent Instructions', link: '/guide/instructions' },
      { text: 'Team and Personal Configuration', link: '/guide/configuration' },
      { text: 'Sandboxes', link: '/guide/sandboxes' },
      { text: 'Ports', link: '/guide/ports' },
      { text: 'Network', link: '/guide/network' },
      { text: 'Secrets', link: '/guide/secrets' },
      { text: 'Doctor', link: '/guide/doctor' },
      { text: 'Keeping the Env Current', link: '/guide/updating' },
    ],
  },
  {
    text: 'Integrations',
    items: [
      { text: 'herdr', link: '/guide/herdr' },
      { text: 'unagit', link: '/guide/unagit' },
    ],
  },
  {
    text: 'More',
    items: [{ text: 'For Env Authors', link: '/guide/env-authors' }],
  },
]

const referenceSidebar = [
  {
    text: 'Reference',
    items: [
      { text: 'Command Line', link: '/reference/cli' },
      { text: 'Files on Disk', link: '/reference/files' },
      { text: 'Security', link: '/reference/security' },
      { text: 'What unibox Asks of sbx', link: '/reference/sbx' },
      { text: 'Developing unibox', link: '/reference/developing' },
    ],
  },
]

// A fenced block of the language term is a terminal session drawn as a
// window: a line that starts with "$ " is a command, and [dim], [ok],
// [err], [warn], [blue], [key], [b] and [sel] ... [/] colour what they
// enclose as unibox does. The info string after term is the window's title.
const termTags = ['dim', 'ok', 'err', 'warn', 'blue', 'key', 'b', 'sel']
function renderTerm(content: string, title: string): string {
  const escape = (t: string) => t.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  const paint = (t: string) =>
    escape(t)
      .replace(new RegExp(`\\[(${termTags.join('|')})\\]`, 'g'), '<span class="t-$1">')
      .replace(/\[\/\]/g, '</span>')
  const lines = content.replace(/\n$/, '').split('\n').map((line) => {
    if (line.startsWith('$ ')) {
      return `<span class="t-line t-cmd"><span class="t-prompt">$</span> <b>${escape(line.slice(2))}</b></span>`
    }
    return `<span class="t-line">${paint(line)}</span>`
  })
  return `<div class="term-shot" v-pre><div class="term-bar"><i></i><i></i><i></i><span>${escape(title || 'terminal')}</span></div><pre class="term-body">${lines.join('')}</pre></div>\n`
}

export default defineConfig({
  base,
  title: 'unibox',
  titleTemplate: ':title | unibox',
  description: 'unibox is a companion to Docker Sandboxes (sbx): tasks, a repository index, agent instructions and interactive lists over the sandboxes your team describes in plain sbx files.',
  cleanUrls: true,
  lastUpdated: true,
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: `${base}favicon.svg` }],
    ['meta', { name: 'theme-color', content: '#0b7a5c' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:site_name', content: 'unibox' }],
    ['meta', { property: 'og:title', content: 'unibox, a companion to Docker Sandboxes' }],
    ['meta', { property: 'og:description', content: 'Tasks, a repository index, agent instructions and interactive lists over plain sbx configuration.' }],
  ],
  markdown: {
    theme: { light: 'github-light', dark: 'github-dark-dimmed' },
    config(md) {
      const fence = md.renderer.rules.fence!
      md.renderer.rules.fence = (tokens, idx, options, env, self) => {
        const [lang, ...title] = tokens[idx].info.trim().split(/\s+/)
        if (lang === 'term') return renderTerm(tokens[idx].content, title.join(' '))
        return fence(tokens, idx, options, env, self)
      }
    },
  },
  themeConfig: {
    logo: { light: '/logo-light.svg', dark: '/logo-dark.svg', alt: 'unibox' },
    siteTitle: false,
    nav: [
      { text: 'Guide', link: '/', activeMatch: '^/($|guide/)' },
      { text: 'Reference', link: '/reference/cli', activeMatch: '^/reference/' },
      {
        text: 'Resources',
        items: [
          { text: 'GitHub', link: repo },
          { text: 'Releases', link: `${repo}/releases` },
          { text: 'Issues', link: `${repo}/issues` },
          { text: 'unagit', link: 'https://bitbyone.github.io/unagit/' },
        ],
      },
    ],
    sidebar: {
      '/reference/': referenceSidebar,
      '/': guideSidebar,
    },
    socialLinks: [{ icon: 'github', link: repo }],
    outline: { level: [2, 3], label: 'On this page' },
    search: { provider: 'local' },
    editLink: {
      pattern: `${repo}/edit/main/website/:path`,
      text: 'Suggest changes to this page',
    },
    footer: {
      message: 'Released under the MIT License.',
      copyright: `© ${new Date().getFullYear()} unibox contributors`,
    },
  },
})
