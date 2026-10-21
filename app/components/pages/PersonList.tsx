import ResourceList from "@/components/library/ResourceList";
import { usePeopleList } from "@/hooks/queries/people";
import { useResourceListState } from "@/hooks/useResourceListState";
import { PERSON_SORT } from "@/libraries/resourceSort";

const PersonList = () => {
  const state = useResourceListState(PERSON_SORT);
  const query = usePeopleList(state.queryParams, {
    enabled: state.pageSizeResolved,
  });

  return (
    <ResourceList
      itemConfig={(person) => {
        const badges = [];
        if (person.authored_book_count > 0) {
          badges.push({
            count: person.authored_book_count,
            singular: "book authored",
            plural: "books authored",
          });
        }
        if (person.narrated_file_count > 0) {
          badges.push({
            count: person.narrated_file_count,
            singular: "file narrated",
            plural: "files narrated",
            variant: "outline" as const,
          });
        }
        if (badges.length === 0) {
          badges.push({
            count: 0,
            singular: "work",
            plural: "works",
            variant: "outline" as const,
          });
        }

        return {
          name: person.name,
          secondaryText:
            person.sort_name !== person.name ? person.sort_name : undefined,
          aliases: person.aliases,
          badges,
        };
      }}
      itemLabel="people"
      linkTo={(person, libraryId) =>
        `/libraries/${libraryId}/people/${person.id}`
      }
      query={query}
      searchLabel="Search people"
      searchPlaceholder="Search by name..."
      state={state}
      subtitle="Authors, narrators, and other contributors"
      title="People"
    />
  );
};

export default PersonList;
