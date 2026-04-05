import Link from "@docusaurus/Link";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";
import {
  Blocks,
  BookOpen,
  FolderOpen,
  Github,
  Grid2x2,
  Layers,
  MonitorSmartphone,
  ScanSearch,
  Shield,
  Users,
} from "lucide-react";
import type { ReactNode } from "react";

const workflowSteps = [
  {
    icon: FolderOpen,
    title: "Point it at your books",
    desc: "Mount a directory of ebooks, audiobooks, or comics. Shisho scans the folder, imports everything it finds automatically, and organizes everything on the file system.",
  },
  {
    icon: ScanSearch,
    title: "Metadata is extracted and enriched",
    desc: "Titles, authors, narrators, series info, covers, and identifiers are pulled from each file. Metadata enricher plugins matches books against online sources to fill in any gaps.",
  },
  {
    icon: MonitorSmartphone,
    title: "Read, listen, or download from anywhere",
    desc: "Browse your library from any device. Read in the browser, sync to your Kobo, connect via OPDS, or download files directly.",
  },
];

const formatCategories = [
  {
    category: "Ebooks",
    formats: [
      { name: "EPUB", planned: false },
      { name: "PDF", planned: false },
      { name: "MOBI", planned: true },
    ],
  },
  {
    category: "Audiobooks",
    formats: [
      { name: "M4B", planned: false },
      { name: "M4A", planned: true },
      { name: "MP3", planned: true },
    ],
  },
  {
    category: "Comics",
    formats: [
      { name: "CBZ", planned: false },
      { name: "CBR", planned: true },
    ],
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
    desc: "Your books stay on your hardware. No cloud, no subscriptions, no data leaving your network. Docker makes setup trivial.",
  },
  {
    icon: Layers,
    title: "Rich Metadata",
    desc: "Automatically extracts titles, authors, narrators, series, covers, genres, and identifiers from every supported format.",
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

const dockerCompose = `services:
  shisho:
    image: ghcr.io/shishobooks/shisho:latest
    container_name: shisho
    restart: unless-stopped
    ports:
      - "5173:5173"
    volumes:
      - ./data:/data
      - ./config:/config
      - /path/to/books:/media
    environment:
      - PUID=1000
      - PGID=1000
      - DATABASE_FILE_PATH=/data/shisho.db
      - JWT_SECRET=your-secret-key`;

function DockerComposeHighlighted(): ReactNode {
  const lines = dockerCompose.split("\n");
  return (
    <>
      <span className="qs-comment"># docker-compose.yml</span>
      {"\n"}
      {lines.map((line, i) => {
        const highlighted = line
          .replace(
            /^(\s*)([\w_]+)(:)/gm,
            (_, indent, key, colon) => `${indent}<k>${key}</k>${colon}`,
          )
          .replace(/(".*?")/g, "<s>$1</s>")
          .replace(
            /(ghcr\.io\/shishobooks\/shisho:latest|unless-stopped)/g,
            "<s>$1</s>",
          );
        return (
          <span key={i}>
            <span
              dangerouslySetInnerHTML={{
                __html: highlighted
                  .replace(/<k>/g, '<span class="qs-key">')
                  .replace(/<\/k>/g, "</span>")
                  .replace(/<s>/g, '<span class="qs-string">')
                  .replace(/<\/s>/g, "</span>"),
              }}
            />
            {"\n"}
          </span>
        );
      })}
    </>
  );
}

export default function Home(): ReactNode {
  const { siteConfig } = useDocusaurusContext();

  return (
    <Layout
      description="Shisho documentation for setup, operation, and architecture."
      title={siteConfig.title}
    >
      <main className="docs-home">
        {/* HERO */}
        <section className="docs-home__hero">
          <div className="docs-home__hero-inner">
            <p className="docs-home__eyebrow">Self-Hosted Book Management</p>
            <h1 className="docs-home__title">
              One library for <em>every</em> book you own
            </h1>
            <p className="docs-home__subtitle">
              Shisho is an open-source, self-hosted system that brings ebooks,
              audiobooks, and comics together in a single unified library. No
              more juggling separate apps.
            </p>
            <div className="docs-home__actions">
              <Link
                className="docs-home__btn docs-home__btn--primary"
                to="/docs/getting-started"
              >
                Get Started
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
            From folder to library in minutes
          </h2>
          <p className="docs-home__section-desc">
            Shisho turns a directory of files into an organized, searchable
            library with rich metadata. No manual cataloging required.
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
            Native support for every book format
          </h2>
          <p className="docs-home__section-desc">
            Full metadata extraction, cover art, and chapter detection. All
            built in, no plugins required.
          </p>
          <div className="docs-home__formats">
            {formatCategories.map((cat) => (
              <div className="docs-home__format-row" key={cat.category}>
                <span className="docs-home__format-category">
                  {cat.category}
                </span>
                <div className="docs-home__format-items">
                  {cat.formats.map((fmt) => (
                    <span
                      className={`docs-home__format-tag${fmt.planned ? " docs-home__format-tag--planned" : ""}`}
                      key={fmt.name}
                    >
                      {fmt.name}
                      {fmt.planned && (
                        <span className="docs-home__format-soon">Soon</span>
                      )}
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
            From metadata to device syncing, Shisho gives you full control over
            your books.
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
                Up and running in minutes
              </h2>
              <p className="docs-home__section-desc docs-home__section-desc--tight">
                Shisho runs as a Docker container. Point it at your book
                collection and you're done.
              </p>
              <div className="docs-home__quickstart-steps">
                <div className="docs-home__quickstart-step">
                  <h4>Create a docker-compose.yml</h4>
                  <p>
                    Copy the configuration and adjust the paths to your book
                    library.
                  </p>
                </div>
                <div className="docs-home__quickstart-step">
                  <h4>Start the container</h4>
                  <p>
                    Run <code>docker compose up -d</code> and visit port 5173.
                  </p>
                </div>
                <div className="docs-home__quickstart-step">
                  <h4>Create a library</h4>
                  <p>
                    Point Shisho at your mounted media directory. It scans
                    automatically.
                  </p>
                </div>
              </div>
            </div>
            <pre className="docs-home__quickstart-code">
              <DockerComposeHighlighted />
            </pre>
          </div>
        </section>

        {/* BOTTOM CTA */}
        <section className="docs-home__section docs-home__section--no-border">
          <div className="docs-home__cta">
            <h2 className="docs-home__cta-heading">
              Ready to organize your library?
            </h2>
            <p className="docs-home__cta-text">
              Shisho is free, open-source, and always will be.
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
