import { Fragment, type ReactNode } from "react";

/** Facts in a quiet line, a dot between each. */
const FactsLine = ({ facts }: { facts: ReactNode[] }) => (
  <>
    {facts.map((fact, i) => (
      <Fragment key={i}>
        {i > 0 && " · "}
        {fact}
      </Fragment>
    ))}
  </>
);

export default FactsLine;
