import { defineConfig } from 'vitepress'

// The site is served by GitHub Pages from the repository's own address,
// https://bitbyone.github.io/unibox/, so every link is under /unibox/.
const base = '/unibox/'
const repo = 'https://github.com/bitbyone/unibox'

const guideSidebar = [
  {
    text: 'Introduction',
    items: [
      { text: 'Getting Started', link: '/guide/' },
      { text: 'How it Works', link: '/guide/how-it-works' },
    ],
  },
  {
    text: 'Daily Work',
    items: [
      { text: 'Tasks', link: '/guide/tasks' },
      { text: 'Sandboxes', link: '/guide/sandboxes' },
      { text: 'Ports', link: '/guide/ports' },
      { text: 'Network', link: '/guide/network' },
      { text: 'Secrets & Agents', link: '/guide/secrets' },
      { text: 'Keeping the Env Current', link: '/guide/updating' },
      { text: 'Troubleshooting', link: '/guide/troubleshooting' },
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

export default defineConfig({
  base,
  title: 'unibox',
  titleTemplate: ':title | unibox',
  description: 'Run Claude Code, Codex, Copilot or OpenCode in a Docker Sandbox, a VM on your machine that gets your repository and nothing else, set up from your team\'s env repository with one command.',
  cleanUrls: true,
  lastUpdated: true,
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: `${base}favicon.svg` }],
    ['meta', { name: 'theme-color', content: '#0b7a5c' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:site_name', content: 'unibox' }],
    ['meta', { property: 'og:title', content: 'unibox: coding agents in a sandbox, not on your laptop' }],
    ['meta', { property: 'og:description', content: 'A CLI over Docker Sandboxes. Your team describes the sandbox once; you run unibox run.' }],
  ],
  markdown: {
    theme: { light: 'github-light', dark: 'github-dark-dimmed' },
  },
  themeConfig: {
    logo: { light: '/logo-light.svg', dark: '/logo-dark.svg', alt: 'unibox' },
    siteTitle: false,
    nav: [
      { text: 'Guide', link: '/guide/', activeMatch: '^/guide/' },
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
      '/guide/': guideSidebar,
      '/reference/': referenceSidebar,
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
