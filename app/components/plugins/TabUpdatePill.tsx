import { Badge } from "@/components/ui/badge";

export const TabUpdatePill = ({ count }: { count: number }) => {
  if (count <= 0) return null;
  const label =
    count === 1
      ? "1 plugin has an update available"
      : `${count} plugins have an update available`;
  return (
    <Badge
      aria-label={label}
      className="ml-2 h-5 min-w-5 bg-primary/20 text-primary border-primary/40"
    >
      {count}
    </Badge>
  );
};
