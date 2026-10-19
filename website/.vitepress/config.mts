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
      { text: 'What unibox Is', link: '/guide/why' },
      { text: 'Concepts', link: '/guide/concepts' },
    ],
  },
  {
    text: 'Environments',
    items: [
      { text: 'The Environments Repository', link: '/guide/environments-repository' },
      { text: 'Profiles', link: '/guide/profiles' },
      { text: 'Images', link: '/guide/images' },
      { text: 'Scripts and Instructions', link: '/guide/scripts-and-instructions' },
    ],
  },
  {
    text: 'Agents and Access',
    items: [
      { text: 'Agents', link: '/guide/agents' },
      { text: 'Services and Secrets', link: '/guide/services-and-secrets' },
      { text: 'Network and Ports', link: '/guide/network-and-ports' },
    ],
  },
  {
    text: 'Working',
    items: [
      { text: 'Tasks', link: '/guide/tasks' },
      { text: 'Mounting a Directory', link: '/guide/mount' },
    ],
  },
  {
    text: 'Integrations',
    items: [
      { text: 'herdr', link: '/guide/herdr' },
      { text: 'unagit', link: '/guide/unagit' },
    ],
  },
]

const referenceSidebar = [
  {
    text: 'Reference',
    items: [
      { text: 'Command Line', link: '/reference/cli' },
      { text: 'sandbox.yaml', link: '/reference/sandbox-yaml' },
      { text: 'Files on Disk', link: '/reference/files' },
      { text: 'Security', link: '/reference/security' },
      { text: 'The sbx Runtime', link: '/reference/sbx' },
      { text: 'The msb Runtime (experimental)', link: '/reference/msb' },
      { text: 'Developing unibox', link: '/reference/developing' },
    ],
  },
]

export default defineConfig({
  base,
  title: 'unibox',
  titleTemplate: ':title | unibox',
  description: 'A command-line wrapper over Docker Sandboxes: one profile format and an easy setup for running coding agents in sandboxes during local development.',
  cleanUrls: true,
  lastUpdated: true,
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: `${base}favicon.svg` }],
    ['meta', { name: 'theme-color', content: '#0b7a5c' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:site_name', content: 'unibox' }],
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
