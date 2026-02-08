import fs from "node:fs";
import path from "node:path";

import type * as Preset from "@docusaurus/preset-classic";
import type { Config } from "@docusaurus/types";
import { themes as prismThemes } from "prism-react-renderer";

const repository = process.env.GITHUB_REPOSITORY ?? "shishobooks/shisho";
const [organizationName, projectName] = repository.split("/");
const versionsFilePath = path.resolve(__dirname, "versions.json");
const releasedDocVersions = fs.existsSync(versionsFilePath)
  ? (JSON.parse(fs.readFileSync(versionsFilePath, "utf8")) as string[])
  : [];
const latestReleasedDocsVersion = releasedDocVersions[0];
const baseUrl = "/";

const config: Config = {
  title: "Shisho",
  tagline: "Your all-in-one solution for ebooks, audiobooks, and comics",
  favicon: "img/favicon.ico",
  future: {
    v4: true,
  },
  url: "https://www.shishobooks.com",
  baseUrl,
  organizationName,
  projectName,
  onBrokenLinks: "throw",
  trailingSlash: false,
  i18n: {
    defaultLocale: "en",
    locales: ["en"],
  },
  stylesheets: [
    "https://fonts.googleapis.com/css2?family=Geist:wght@100..900&family=Noto+Sans+JP:wght@100..900&family=Noto+Sans:ital,wght@0,100..900;1,100..900&display=swap",
  ],
  presets: [
    [
      "classic",
      {
        docs: {
          routeBasePath: "docs",
          sidebarPath: "./sidebars.ts",
          editUrl: "https://github.com/shishobooks/shisho/tree/master/website/",
          lastVersion: latestReleasedDocsVersion ?? "current",
          versions: {
            current: {
              label: "Unreleased",
              path: "unreleased",
              banner: "unreleased",
            },
          },
        },
        blog: false,
        theme: {
          customCss: "./src/css/custom.css",
        },
      } satisfies Preset.Options,
    ],
  ],
  themeConfig: {
    image: "img/shisho-social-card.png",
    colorMode: {
      defaultMode: "dark",
      disableSwitch: true,
      respectPrefersColorScheme: false,
    },
    navbar: {
      title: "Shisho",
      logo: {
        alt: "Shisho",
        src: "img/logo-mark.svg",
      },
      items: [
        {
          type: "docSidebar",
          sidebarId: "docsSidebar",
          position: "left",
          label: "Docs",
        },
        {
          href: "https://github.com/shishobooks/shisho",
          label: "GitHub",
          position: "right",
        },
        {
          type: "docsVersionDropdown",
          position: "right",
          dropdownActiveClassDisabled: true,
        },
      ],
    },
    footer: {
      style: "dark",
      links: [
        {
          title: "Docs",
          items: [
            {
              label: "Getting Started",
              to: "/docs/getting-started",
            },
          ],
        },
        {
          title: "Project",
          items: [
            {
              label: "Releases",
              href: "https://github.com/shishobooks/shisho/releases",
            },
            {
              label: "Issues",
              href: "https://github.com/shishobooks/shisho/issues",
            },
          ],
        },
        {
          title: "Support",
          items: [
            {
              label: "Patreon",
              href: "https://www.patreon.com/shishobooks",
            },
            {
              label: "GitHub Sponsors",
              href: "https://github.com/sponsors/robinjoseph08",
            },
          ],
        },
      ],
      copyright: `Copyright \u00a9 ${new Date().getFullYear()} Shisho`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
