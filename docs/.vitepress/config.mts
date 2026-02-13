import { withMermaid } from "vitepress-plugin-mermaid";
import { fileURLToPath, URL } from 'node:url'

// https://vitepress.dev/reference/site-config
export default withMermaid({
  title: "Metal Operator",
  description: "Kubernetes Operator to manage Bare Metal Servers",
  base: "/metal-operator/",
  head: [['link', { rel: 'icon', href: 'https://raw.githubusercontent.com/ironcore-dev/ironcore/refs/heads/main/docs/assets/logo_borderless.svg' }]],
  vite: {
      resolve: {
          alias: [
              {
                  find: /^.*\/VPFooter\.vue$/,
                  replacement: fileURLToPath(
                      new URL('./theme/components/VPFooter.vue', import.meta.url)
                  )
              },
          ]
      }
  },
  themeConfig: {
    // https://vitepress.dev/reference/default-theme-config
    nav: [
      { text: 'Home', link: '/' },
      { text: 'Documentation', link: '/architecture' },
      { text: 'Quickstart', link: '/quickstart' },
      { text: 'IronCore Documentation', link: 'https://ironcore-dev.github.io' },
    ],

    editLink: {
      pattern: 'https://github.com/ironcore-dev/metal-operator/blob/main/docs/:path',
      text: 'Edit this page on GitHub'
    },

    logo: {
        src: 'https://raw.githubusercontent.com/ironcore-dev/ironcore/refs/heads/main/docs/assets/logo_borderless.svg',
        width: 24,
        height: 24
    },

    search: {
      provider: 'local'
    },

    sidebar: [
        {
        items: [
          { text: "Quickstart", link: '/quickstart' },
          { text: 'Installation',
            collapsed: true,
            items: [
              { text: 'Kustomize', link: '/installation/kustomize' },
              { text: 'Helm', link: '/installation/helm' },
            ]
          },
          { text: 'Architecture', link: '/architecture' },
          { text: 'API Reference', link: '/api-reference/api' },
        ]
      },
      {
        text: 'Concepts',
        collapsed: false,
        items: [
          { text: 'Endpoints', link: '/concepts/endpoints' },
          { text: 'BMCs', link: '/concepts/bmcs' },
          { text: 'BMCSecrets', link: '/concepts/bmcsecrets' },
          { text: 'BMCSettings', link: '/concepts/bmcsettings' },
          { text: 'BMCVersion', link: '/concepts/bmcversion' },
          { text: 'BMCVersionSet', link: '/concepts/bmcversionset' },
          { text: 'Servers', link: '/concepts/servers' },
          { text: 'ServerClaims', link: '/concepts/serverclaims' },
          { text: 'ServerBootConfigurations', link: '/concepts/serverbootconfigurations' },
          { text: 'ServerMaintenance', link: '/concepts/servermaintenance' },
          { text: 'BIOSSettings', link: '/concepts/biossettings' },
          { text: 'BIOSSettingsSet', link: '/concepts/biossettingsset' },
          { text: 'BIOSVersion', link: '/concepts/biosversion' },
          { text: 'BIOSVersionSet', link: '/concepts/biosversionset' },
        ]
      },
      {
        text: 'Usage',
        collapsed: false,
        items: [
          { text: 'metalctl', link: '/usage/metalctl' },
        ]
      },
      {
        text: 'Development',
        collapsed: false,
        items: [
          { text: 'Local Dev Setup', link: '/development/dev_setup' },
          { text: 'Documentation', link: '/development/dev_docs' },
        ]
      },
      {
        text: 'Contributing',
        collapsed: false,
        items: [
          { text: 'Contributing Guide', link: 'https://ironcore.dev/community/contributing.html' },
          { text: 'Cherry-Picking', link: '/contributing/cherry_pick' },
        ]
      }
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/ironcore-dev/metal-operator' }
    ],
  }
})
