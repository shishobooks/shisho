import Link from "@docusaurus/Link";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import {
  Blocks,
  BookOpen,
  FolderOpen,
  Grid2x2,
  Layers,
  MonitorSmartphone,
  ScanSearch,
  Shield,
  Users,
} from "lucide-react";
import type { ReactNode } from "react";

import { Github } from "../components/GithubIcon";

const workflowSteps = [
  {
    icon: FolderOpen,
    title: "Mount your supported books",
    desc: "Mount a directory of supported ebooks, audiobooks, or comics. Shisho scans the folder and imports compatible files. Moving and renaming files is an optional library setting.",
  },
  {
    icon: ScanSearch,
    title: "Metadata is extracted and enriched",
    desc: "Shisho extracts format-specific metadata from each file. Optional metadata enricher plugins can match books against online sources.",
  },
  {
    icon: MonitorSmartphone,
    title: "Read, listen, or download",
    desc: "Use the browser, Kobo sync, OPDS, or direct downloads from devices that can reach your Shisho server.",
  },
];

const formatCategories = [
  {
    category: "Ebooks",
    formats: [{ name: "EPUB" }, { name: "PDF" }],
  },
  {
    category: "Audiobooks",
    formats: [{ name: "M4B" }],
  },
  {
    category: "Comics",
    formats: [{ name: "CBZ" }],
  },
];

const features = [
  {
    icon: Grid2x2,
    title: "Unified Library",
    desc: "Ebooks, audiobooks, and comics all live together. Browse by series, author, genre, or format from one interface.",
  },
  {
    icon: Shield,
    title: "Self-Hosted & Private",
    desc: "Shisho runs on hardware you control with no required subscription. Enabled plugins and integrations may connect to services you configure.",
  },
  {
    icon: Layers,
    title: "Rich Metadata",
    desc: "Extract format-specific titles, contributors, series, covers, languages, chapters, and identifiers, then review or edit them.",
  },
  {
    icon: BookOpen,
    title: "Kobo Sync & OPDS",
    desc: "Sync books directly to your Kobo e-reader with automatic KePub conversion. OPDS catalog for any compatible reader app.",
  },
  {
    icon: Users,
    title: "Multi-User & Permissions",
    desc: "Create users with Admin, Editor, or Viewer roles. Control access per library with fine-grained permissions.",
  },
  {
    icon: Blocks,
    title: "Plugin System",
    desc: "Extend functionality with JavaScript plugins for format conversion, metadata enrichment, and custom integrations.",
  },
];

export default function Home(): ReactNode {
  const { siteConfig } = useDocusaurusContext();

  return (
    <Layout
      description="Shisho documentation for evaluation, setup, operation, integrations, and plugin development."
      title={siteConfig.title}
    >
      <main className="docs-home">
        {/* HERO */}
        <section className="docs-home__hero">
          <div className="docs-home__hero-inner">
            <p className="docs-home__eyebrow">Self-Hosted Book Management</p>
            <h1 className="docs-home__title">
              One library for your <em>digital</em> books
            </h1>
            <p className="docs-home__subtitle">
              Shisho is an open-source, self-hosted system that brings supported
              ebooks, audiobooks, and comics together in a unified library.
            </p>
            <div className="docs-home__actions">
              <Link
                className="docs-home__btn docs-home__btn--primary"
                to="/docs/getting-started"
              >
                Get Started
              </Link>
              <Link
                className="docs-home__btn docs-home__btn--ghost"
                to="/docs/supported-formats"
              >
                Check Formats
              </Link>
              <a
                className="docs-home__btn docs-home__btn--ghost"
                href="https://github.com/shishobooks/shisho"
                rel="noopener noreferrer"
                target="_blank"
              >
                <Github size={16} strokeWidth={2} />
                View on GitHub
              </a>
            </div>
          </div>
        </section>

        {/* HOW IT WORKS */}
        <section className="docs-home__section">
          <p className="docs-home__section-label">How It Works</p>
          <h2 className="docs-home__section-heading">
            From supported files to a searchable library
          </h2>
          <p className="docs-home__section-desc">
            Shisho scans compatible files, extracts available metadata, and
            gives you tools to review and manage the resulting library.
          </p>
          <div className="docs-home__workflow">
            {workflowSteps.map((step, i) => (
              <div className="docs-home__workflow-step" key={step.title}>
                <div className="docs-home__workflow-number">{i + 1}</div>
                <div className="docs-home__workflow-icon">
                  <step.icon size={22} />
                </div>
                <h3 className="docs-home__workflow-title">{step.title}</h3>
                <p className="docs-home__workflow-desc">{step.desc}</p>
              </div>
            ))}
          </div>
        </section>

        {/* FORMATS */}
        <section className="docs-home__section">
          <p className="docs-home__section-label">Format Support</p>
          <h2 className="docs-home__section-heading">
            Native support for popular book formats
          </h2>
          <p className="docs-home__section-desc">
            Capabilities vary by format, including metadata extraction, browser
            reading, and generated downloads. See the complete{" "}
            <Link to="/docs/supported-formats">Supported Formats</Link> guide.
          </p>
          <div className="docs-home__formats">
            {formatCategories.map((cat) => (
              <div className="docs-home__format-row" key={cat.category}>
                <span className="docs-home__format-category">
                  {cat.category}
                </span>
                <div className="docs-home__format-items">
                  {cat.formats.map((fmt) => (
                    <span className="docs-home__format-tag" key={fmt.name}>
                      {fmt.name}
                    </span>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </section>

        {/* FEATURES */}
        <section className="docs-home__section">
          <p className="docs-home__section-label">Features</p>
          <h2 className="docs-home__section-heading">
            Everything you need to manage your library
          </h2>
          <p className="docs-home__section-desc">
            Choose the guide for your task:{" "}
            <Link to="/docs/metadata">use Shisho</Link>,{" "}
            <Link to="/docs/configuration">administer a deployment</Link>,
            connect through <Link to="/docs/opds">OPDS</Link>,{" "}
            <Link to="/docs/kobo-sync">Kobo Sync</Link>, or the{" "}
            <Link to="/docs/ereader-browser">eReader Browser</Link>, or{" "}
            <Link to="/docs/plugins/development">develop plugins</Link>.
          </p>
          <div className="docs-home__features">
            {features.map((feat) => (
              <article className="docs-home__feature" key={feat.title}>
                <div className="docs-home__feature-icon">
                  <feat.icon size={18} strokeWidth={2} />
                </div>
                <h3 className="docs-home__feature-title">{feat.title}</h3>
                <p className="docs-home__feature-desc">{feat.desc}</p>
              </article>
            ))}
          </div>
        </section>

        {/* QUICKSTART */}
        <section className="docs-home__section">
          <div className="docs-home__quickstart">
            <div>
              <p className="docs-home__section-label">Quick Start</p>
              <h2 className="docs-home__section-heading">
                Start with the deployment checklist
              </h2>
              <p className="docs-home__section-desc docs-home__section-desc--tight">
                Shisho runs as a Docker container. Before starting, review the{" "}
                <Link to="/docs/getting-started">
                  complete deployment steps
                </Link>{" "}
                for persistent storage, a strong JWT secret, and host file
                permissions.
              </p>
              <div className="docs-home__quickstart-steps">
                <div className="docs-home__quickstart-step">
                  <h4>Review and adapt the compose file</h4>
                  <p>
                    Set persistent paths and mount the library at the container
                    path you plan to configure in Shisho.
                  </p>
                </div>
                <div className="docs-home__quickstart-step">
                  <h4>Set the secret and permissions</h4>
                  <p>
                    Generate a JWT secret and match PUID and PGID to the host
                    account that owns the mounted files.
                  </p>
                </div>
                <div className="docs-home__quickstart-step">
                  <h4>Start Shisho and create a library</h4>
                  <p>
                    Start the container, visit port 5173, and create a library
                    using the mounted container path.
                  </p>
                </div>
              </div>
            </div>
          </div>
        </section>

        {/* BOTTOM CTA */}
        <section className="docs-home__section docs-home__section--no-border">
          <div className="docs-home__cta">
            <h2 className="docs-home__cta-heading">
              Ready to build your Shisho library?
            </h2>
            <p className="docs-home__cta-text">
              Shisho is free and open source.
            </p>
            <div className="docs-home__actions">
              <Link
                className="docs-home__btn docs-home__btn--primary"
                to="/docs/getting-started"
              >
                Read the Docs
              </Link>
              <a
                className="docs-home__btn docs-home__btn--ghost"
                href="https://www.patreon.com/shishobooks"
                rel="noopener noreferrer"
                target="_blank"
              >
                Support on Patreon
              </a>
            </div>
          </div>
        </section>
      </main>
    </Layout>
  );
}
