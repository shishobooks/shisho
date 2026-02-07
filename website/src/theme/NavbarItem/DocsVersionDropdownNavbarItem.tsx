import {
  useActiveDocContext,
  useDocsPreferredVersion,
  useDocsVersionCandidates,
  useVersions,
} from "@docusaurus/plugin-content-docs/client";
import { useHistorySelector } from "@docusaurus/theme-common";
import { translate } from "@docusaurus/Translate";
import DefaultNavbarItem from "@theme/NavbarItem/DefaultNavbarItem";
import DropdownNavbarItem from "@theme/NavbarItem/DropdownNavbarItem";
import { ChevronDown } from "lucide-react";
import type { ReactNode } from "react";

type Version = {
  name: string;
  label: string;
  mainDocId: string;
  docs: { id: string; path: string }[];
};

type LinkLikeItem = {
  label: string;
  to: string;
  isActive: () => boolean;
  onClick: () => void;
};

type VersionItem = {
  version: Version;
  label: string;
};

type ActiveDocContextLike = {
  activeVersion?: Version;
  alternateDocVersions: Record<string, { path: string } | undefined>;
};

type DocsVersionDropdownNavbarItemProps = {
  mobile?: boolean;
  docsPluginId?: string;
  dropdownActiveClassDisabled?: boolean;
  dropdownItemsBefore?: LinkLikeItem[];
  dropdownItemsAfter?: LinkLikeItem[];
  versions?: Record<string, { label?: string }> | string[];
  [key: string]: unknown;
};

function getVersionItems(
  versions: Version[],
  configs?: Record<string, { label?: string }> | string[],
): VersionItem[] {
  if (!configs) {
    return versions.map((version) => ({ version, label: version.label }));
  }

  const versionMap = new Map(
    versions.map((version) => [version.name, version]),
  );
  const toVersionItem = (name: string, config?: { label?: string }) => {
    const version = versionMap.get(name);
    if (!version) {
      throw new Error(
        `No docs version for '${name}'. Available: ${versions
          .map((v) => v.name)
          .join(", ")}`,
      );
    }
    return { version, label: config?.label ?? version.label };
  };

  if (Array.isArray(configs)) {
    return configs.map((name) => toVersionItem(name));
  }

  return Object.entries(configs).map(([name, config]) =>
    toVersionItem(name, config),
  );
}

function getVersionMainDoc(version: Version) {
  return version.docs.find((doc) => doc.id === version.mainDocId);
}

function getVersionTargetDoc(
  version: Version,
  activeDocContext: ActiveDocContextLike,
) {
  return (
    activeDocContext.alternateDocVersions[version.name] ??
    getVersionMainDoc(version)
  );
}

function useDisplayedVersionItem({
  docsPluginId,
  versionItems,
}: {
  docsPluginId?: string;
  versionItems: VersionItem[];
}) {
  const candidates = useDocsVersionCandidates(docsPluginId);
  const candidateItems = candidates
    .map((candidate) => versionItems.find((item) => item.version === candidate))
    .filter((item): item is VersionItem => item !== undefined);
  return candidateItems[0] ?? versionItems[0];
}

export default function DocsVersionDropdownNavbarItem({
  mobile,
  docsPluginId,
  dropdownActiveClassDisabled,
  dropdownItemsBefore = [],
  dropdownItemsAfter = [],
  versions: configs,
  ...props
}: DocsVersionDropdownNavbarItemProps): ReactNode {
  const search = useHistorySelector((history) => history.location.search);
  const hash = useHistorySelector((history) => history.location.hash);
  const activeDocContext = useActiveDocContext(docsPluginId);
  const { savePreferredVersionName } = useDocsPreferredVersion(docsPluginId);

  const versions = useVersions(docsPluginId) as Version[];
  const versionItems = getVersionItems(versions, configs);
  const displayedVersionItem = useDisplayedVersionItem({
    docsPluginId,
    versionItems,
  });

  function versionItemToLink({ version, label }: VersionItem) {
    const targetDoc = getVersionTargetDoc(version, activeDocContext);
    return {
      label,
      to: `${targetDoc.path}${search}${hash}`,
      isActive: () => version === activeDocContext.activeVersion,
      onClick: () => savePreferredVersionName(version.name),
    };
  }

  const items: LinkLikeItem[] = [
    ...dropdownItemsBefore,
    ...versionItems.map(versionItemToLink),
    ...dropdownItemsAfter,
  ];

  const dropdownLabel =
    mobile && items.length > 1
      ? translate({
          id: "theme.navbar.mobileVersionsDropdown.label",
          message: "Versions",
          description: "The label for mobile docs versions dropdown",
        })
      : displayedVersionItem.label;

  const dropdownTo =
    mobile && items.length > 1
      ? undefined
      : getVersionTargetDoc(displayedVersionItem.version, activeDocContext)
          .path;

  if (items.length <= 1) {
    return (
      <DefaultNavbarItem
        {...props}
        isActive={dropdownActiveClassDisabled ? () => false : undefined}
        label={dropdownLabel}
        mobile={mobile}
        to={dropdownTo}
      />
    );
  }

  return (
    <DropdownNavbarItem
      {...props}
      isActive={dropdownActiveClassDisabled ? () => false : undefined}
      items={items}
      label={dropdownLabel}
      mobile={mobile}
      to={dropdownTo}
    >
      <span>{dropdownLabel}</span>
      {!mobile && (
        <ChevronDown
          aria-hidden
          className="navbar-dropdown-chevron"
          size={16}
          strokeWidth={2.1}
        />
      )}
    </DropdownNavbarItem>
  );
}
