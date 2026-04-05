export type FieldStatus = "unchanged" | "changed" | "new";

export interface IdentifierEntry {
  type: string;
  value: string;
}

export function resolveIdentifiers(
  current: IdentifierEntry[],
  incoming: IdentifierEntry[],
): { value: IdentifierEntry[]; status: FieldStatus } {
  if (current.length === 0 && incoming.length > 0)
    return { value: incoming, status: "new" };
  if (current.length > 0 && incoming.length === 0)
    return { value: current, status: "unchanged" };
  const key = (id: IdentifierEntry) => `${id.type}:${id.value}`;
  const curKeys = current.map(key).sort();
  const incKeys = incoming.map(key).sort();
  if (
    curKeys.length === incKeys.length &&
    curKeys.every((v, i) => v === incKeys[i])
  ) {
    return { value: current, status: "unchanged" };
  }
  // Merge: keep all current, add new incoming identifiers
  const existingKeys = new Set(current.map(key));
  const newFromIncoming = incoming.filter((id) => !existingKeys.has(key(id)));
  if (newFromIncoming.length === 0) {
    // Incoming is a subset of current — nothing new to add
    return { value: current, status: "unchanged" };
  }
  return { value: [...current, ...newFromIncoming], status: "changed" };
}
