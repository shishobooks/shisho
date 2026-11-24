import { formatExactWordCount, formatWordCount } from "@/utils/format";

/**
 * A word count in the ballpark, "29K words", with the exact count on hover
 * once it's rounded.
 */
const WordCount = ({ count }: { count: number }) => {
  const shown = formatWordCount(count);
  const exact = formatExactWordCount(count);
  return <span title={shown === exact ? undefined : exact}>{shown}</span>;
};

export default WordCount;
