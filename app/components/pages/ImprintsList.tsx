import ResourceList from "@/components/library/ResourceList";
import { useImprintsList } from "@/hooks/queries/imprints";
import { useResourceListState } from "@/hooks/useResourceListState";
import type { Imprint } from "@/types";

const ImprintsList = () => {
  const state = useResourceListState();
  const query = useImprintsList(state.queryParams);

  return (
    <ResourceList<Imprint>
      itemConfig={(imprint) => {
        const fileCount = imprint.file_count ?? 0;
        return {
          name: imprint.name,
          aliases: imprint.aliases.map((a) => a.name),
          badges: [
            {
              label: fileCount === 1 ? "file" : "files",
              count: fileCount,
            },
          ],
        };
      }}
      itemLabel="imprints"
      linkTo={(imprint, libraryId) =>
        `/libraries/${libraryId}/imprints/${imprint.id}`
      }
      query={query}
      searchPlaceholder="Search imprints..."
      state={state}
      subtitle="Browse imprints in your library"
      title="Imprints"
    />
  );
};

export default ImprintsList;
