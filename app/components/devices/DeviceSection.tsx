/**
 * DeviceSection is one titled, bordered section of a Device's page.
 */
const DeviceSection = ({
  id,
  title,
  description,
  children,
}: {
  id: string;
  title: string;
  description?: string;
  children: React.ReactNode;
}) => (
  <section className="rounded-md border border-border p-4 md:p-6">
    <h2 className="text-lg font-semibold" id={id}>
      {title}
    </h2>
    {description && (
      <p className="mt-1 text-sm text-muted-foreground">{description}</p>
    )}
    <div className="mt-4 space-y-4">{children}</div>
  </section>
);

export default DeviceSection;
