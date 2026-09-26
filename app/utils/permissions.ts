import type { EntityType } from "@/libraries/metadataEntity";
import { ResourceBooks, ResourcePeople, ResourceSeries } from "@/types";

/**
 * The permission resource whose Write operation the backend requires for
 * mutating a metadata entity. Genres, tags, and publishers are edited under
 * the books route group, so they require Books Write rather than a resource
 * of their own.
 */
const WRITE_RESOURCE_BY_ENTITY: Record<EntityType, string> = {
  genre: ResourceBooks,
  tag: ResourceBooks,
  publisher: ResourceBooks,
  series: ResourceSeries,
  person: ResourcePeople,
};

export function writeResourceForEntity(entityType: EntityType): string {
  return WRITE_RESOURCE_BY_ENTITY[entityType];
}
