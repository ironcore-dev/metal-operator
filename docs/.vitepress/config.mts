import { withMermaid } from "vitepress-plugin-mermaid";

// https://vitepress.dev/reference/site-config
export default withMermaid({
  title: "Metal Operator",
  description: "Kubernetes Operator to manage Bare Metal Servers",
  base: "/metal-operator/",
  head: [['link', { rel: 'icon', href: 'https://raw.githubusercontent.com/ironcore-dev/ironcore/refs/heads/main/docs/assets/logo_borderless.svg' }]],
  themeConfig: {
    // https://vitepress.dev/reference/default-theme-config
    nav: [
      { text: 'Home', link: '/' },
      { text: 'Concepts', link: '/concepts' },
      { text: 'Usage', link: '/usage' },
      { text: 'IronCore Documentation', link: 'https://ironcore-dev.github.io' },
    ],

    editLink: {
      pattern: 'https://github.com/ironcore-dev/metal-operator/blob/main/docs/:path',
      text: 'Edit this page on GitHub'
    },

    footer: {
      copyright: 'Copyright © Linux Foundation Europe. IronCore is a project of Linux Foundation Europe. For applicable' +
          ' policies including privacy policy, terms of use and trademark usage guidelines, please see ' +
          '<a href=https://linuxfoundation.eu>https://linuxfoundation.eu</a>. Linux is a registered trademark of ' +
          'Linus Torvalds.'
    },

    logo: { src: 'https://raw.githubusercontent.com/ironcore-dev/ironcore/refs/heads/main/docs/assets/logo_borderless.svg', width: 24, height: 24 },

    search: {
      provider: 'local'
    },

    sidebar: [
        {
        items: [
          { text: 'Quick Start', link: '/usage/installation' },
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
          { text: 'BIOSVersion', link: '/concepts/biosversion' },
          { text: 'BIOSVersionSet', link: '/concepts/biosversionset' },
        ]
      },
      {
        text: 'Usage',
        collapsed: false,
        items: [
          { text: 'Installation', link: '/usage/installation' },
          { text: 'metalctl', link: '/usage/metalctl' },
        ]
      },
      {
        text: 'Developer Guide',
        collapsed: false,
        items: [
          { text: 'Local Dev Setup', link: '/development/dev_setup' },
          { text: 'Documentation', link: '/development/dev_docs' },
        ]
      }
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/ironcore-dev/metal-operator' }
    ],
  }
})
