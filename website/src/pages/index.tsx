import Link from "@docusaurus/Link";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import ShishoLogo from "@site/src/components/ShishoLogo";
import Layout from "@theme/Layout";
import type { ReactNode } from "react";

const highlights = [
  {
    title: "All Your Books, One Place",
    description:
      "Manage ebooks (EPUB), audiobooks (M4B), and comics (CBZ) in a single unified library. No more juggling separate apps for different formats.",
  },
  {
    title: "Self-Hosted",
    description:
      "Your books, your server. Run Shisho on your own hardware with Docker. No cloud dependencies, no subscriptions, no data leaving your network.",
  },
  {
    title: "Extensible with Plugins",
    description:
      "Extend Shisho with JavaScript plugins to customize metadata processing, add integrations, and tailor the system to your workflow.",
  },
];

export default function Home(): ReactNode {
  const { siteConfig } = useDocusaurusContext();

  return (
    <Layout
      description="Shisho documentation for setup, operation, and architecture."
      title={siteConfig.title}
    >
      <main className="docs-home">
        <section className="docs-home__hero">
          <div className="docs-home__hero-inner">
            <ShishoLogo className="docs-home__logo" size="lg" />
            <p className="docs-home__subtitle">{siteConfig.tagline}</p>
            <div className="docs-home__actions">
              <Link
                className="button button--primary button--lg docs-home__button"
                to="/docs/getting-started"
              >
                Getting Started
              </Link>
            </div>
          </div>
        </section>
        <section className="docs-home__highlights container">
          {highlights.map((highlight) => (
            <article className="docs-home__card" key={highlight.title}>
              <h2>{highlight.title}</h2>
              <p>{highlight.description}</p>
            </article>
          ))}
        </section>
      </main>
    </Layout>
  );
}
