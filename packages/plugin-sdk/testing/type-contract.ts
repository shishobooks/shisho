import type { ParsedMetadata } from "../index";

type Equal<Left, Right> =
  (<Type>() => Type extends Left ? 1 : 2) extends <Type>() => Type extends Right
    ? 1
    : 2
    ? true
    : false;
type Expect<Type extends true> = Type;

type SeriesNumberEndContract = Expect<
  Equal<ParsedMetadata["seriesNumberEnd"], number | undefined>
>;

export type { SeriesNumberEndContract };
