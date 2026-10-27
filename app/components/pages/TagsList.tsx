import { useSearchParams } from "react-router-dom";

import ResourceList from "@/components/library/ResourceList";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useUserLibrary } from "@/hooks/queries/libraries";
import { useTagNamespaceCounts, useTagsList } from "@/hooks/queries/tags";
import { useResourceListState } from "@/hooks/useResourceListState";
import { BOOK_COUNT_RESOURCE_SORT } from "@/libraries/resourceSort";
import { splitTagNamespace } from "@/utils/tagNamespaces";

// Tab values. A namespace holds no colon, so "ns:" values never collide
// with the plain tab's.
const PLAIN_TAB = "plain";
const namespaceTab = (namespace: string) => `ns:${namespace}`;

const TagsList = () => {
  const state = useResourceListState(BOOK_COUNT_RESOURCE_SORT);
  const [searchParams, setSearchParams] = useSearchParams();
  const library = useUserLibrary(state.libraryId);
  const namespaces = library.data?.tag_namespaces ?? [];
  const tabbed = namespaces.length > 0;

  // The tab deep-links through ?namespace= rather than a route param, since
  // /tags/:id holds that path segment. A namespace from an old link, or one
  // since removed from the library's settings, shows the plain tags.
  const requested = searchParams.get("namespace");
  const namespace =
    requested && namespaces.includes(requested) ? requested : null;

  const query = useTagsList(
    {
      ...state.queryParams,
      ...(namespace
        ? { namespace }
        : tabbed
          ? { exclude_namespaces: true }
          : {}),
    },
    {
      // Wait for the library so the list is not fetched unsplit first.
      enabled: state.pageSizeResolved && !library.isPending,
    },
  );
  const counts = useTagNamespaceCounts(
    {
      library_id: Number(state.libraryId),
      search: state.searchQuery || undefined,
    },
    { enabled: tabbed },
  );
  const countFor = (ns: string | null) =>
    counts.data?.find((c) => c.namespace === ns)?.total;

  const selectTab = (value: string) => {
    const params = new URLSearchParams(searchParams);
    if (value === PLAIN_TAB) params.delete("namespace");
    else params.set("namespace", value.slice("ns:".length));
    // A new tab starts at its first page.
    params.delete("page");
    params.delete("start");
    setSearchParams(params, { preventScrollReset: true });
  };

  const tabs = tabbed && (
    <Tabs
      onValueChange={selectTab}
      value={namespace ? namespaceTab(namespace) : PLAIN_TAB}
    >
      <TabsList className="w-full justify-start overflow-x-auto overflow-y-hidden">
        {[null, ...namespaces].map((ns) => {
          const count = countFor(ns);
          return (
            <TabsTrigger
              key={ns ?? PLAIN_TAB}
              value={ns ? namespaceTab(ns) : PLAIN_TAB}
            >
              {ns ?? "Tags"}
              {count !== undefined && (
                <span className="ml-1.5 text-xs font-normal text-muted-foreground">
                  {count.toLocaleString()}
                </span>
              )}
            </TabsTrigger>
          );
        })}
      </TabsList>
    </Tabs>
  );

  return (
    <ResourceList
      itemConfig={(tag) => {
        const bookCount = tag.book_count ?? 0;
        return {
          // A namespace tab shows its tags without the prefix.
          name: splitTagNamespace(tag.name, namespaces).name,
          aliases: tag.aliases,
          previewBooks: tag.preview_books,
          badges: [
            {
              count: bookCount,
              singular: "book",
              plural: "books",
            },
          ],
        };
      }}
      itemLabel="tags"
      linkTo={(tag, libraryId) => `/libraries/${libraryId}/tags/${tag.id}`}
      query={query}
      searchLabel={namespace ? `Search ${namespace}` : "Search tags"}
      state={state}
      subtitle="Browse tags in your library"
      tabs={tabs}
      title="Tags"
    />
  );
};

export default TagsList;
