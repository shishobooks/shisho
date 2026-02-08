import isInternalUrl from "@docusaurus/isInternalUrl";
import Link from "@docusaurus/Link";
import { isRegexpStringMatch } from "@docusaurus/theme-common";
import useBaseUrl from "@docusaurus/useBaseUrl";
import IconExternalLink from "@theme/Icon/ExternalLink";
import { BookOpenText, ChevronDown, Github } from "lucide-react";
import type { ReactNode } from "react";

interface NavbarNavLinkProps {
  activeBasePath?: string;
  activeBaseRegex?: string;
  to?: string;
  href?: string;
  label?: string;
  html?: string;
  isDropdownLink?: boolean;
  prependBaseUrlToHref?: boolean;
  [key: string]: unknown;
}

function shouldShowGithubIcon(label?: string, href?: string) {
  if (!label || !href) return false;
  return label.toLowerCase() === "github" && href.includes("github.com");
}

function shouldShowDocsIcon(label?: string, to?: string, href?: string) {
  if (!label) return false;
  if (label.toLowerCase() !== "docs") return false;
  return Boolean(to?.includes("/docs") || href?.includes("/docs"));
}

export default function NavbarNavLink({
  activeBasePath,
  activeBaseRegex,
  to,
  href,
  label,
  html,
  isDropdownLink,
  prependBaseUrlToHref,
  ...props
}: NavbarNavLinkProps): ReactNode {
  const toUrl = useBaseUrl(to);
  const activeBaseUrl = useBaseUrl(activeBasePath);
  const normalizedHref = useBaseUrl(href, { forcePrependBaseUrl: true });
  const isExternalLink = Boolean(label && href && !isInternalUrl(href));
  const showGithubIcon = shouldShowGithubIcon(label, href);
  const showDocsIcon = shouldShowDocsIcon(label, to, href);
  const hasPopup = props["aria-haspopup"];
  const showDropdownChevron = Boolean(
    !isDropdownLink &&
    (hasPopup === true ||
      hasPopup === "true" ||
      hasPopup === "menu" ||
      hasPopup === "listbox"),
  );

  const linkContentProps = html
    ? { dangerouslySetInnerHTML: { __html: html } }
    : {
        children: (
          <>
            {showGithubIcon && (
              <Github
                aria-hidden
                className="navbar-github-icon"
                size={16}
                strokeWidth={2.1}
              />
            )}
            {showDocsIcon && (
              <BookOpenText
                aria-hidden
                className="navbar-docs-icon"
                size={16}
                strokeWidth={2.1}
              />
            )}
            {label}
            {showDropdownChevron && (
              <ChevronDown
                aria-hidden
                className="navbar-dropdown-chevron"
                size={16}
                strokeWidth={2.1}
              />
            )}
            {isExternalLink && (
              <IconExternalLink
                {...(isDropdownLink && { width: 12, height: 12 })}
              />
            )}
          </>
        ),
      };

  if (href) {
    return (
      <Link
        href={prependBaseUrlToHref ? normalizedHref : href}
        {...props}
        {...linkContentProps}
      />
    );
  }

  return (
    <Link
      isNavLink
      to={toUrl}
      {...((activeBasePath || activeBaseRegex) && {
        isActive: (_match: unknown, location: { pathname: string }) =>
          activeBaseRegex
            ? isRegexpStringMatch(activeBaseRegex, location.pathname)
            : location.pathname.startsWith(activeBaseUrl),
      })}
      {...props}
      {...linkContentProps}
    />
  );
}
