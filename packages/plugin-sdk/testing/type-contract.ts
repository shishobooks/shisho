import type { OutputGeneratorContext, ParsedMetadata } from "../index";

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

type OutputSeriesContract = Expect<
  Equal<
    OutputGeneratorContext["book"]["series"],
    Array<{ name: string; number?: number }> | undefined
  >
>;

type OutputFilepathContract = Expect<
  Equal<OutputGeneratorContext["file"]["filepath"], string | undefined>
>;

export type {
  OutputFilepathContract,
  OutputSeriesContract,
  SeriesNumberEndContract,
};
