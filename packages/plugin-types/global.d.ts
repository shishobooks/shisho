import { ShishoPlugin } from "./hooks";
import { ShishoHostAPI } from "./host-api";

declare global {
  /** Host API object providing logging, config, HTTP, filesystem, archive, XML, and FFmpeg access. */
  // eslint-disable-next-line no-var
  var shisho: ShishoHostAPI;
  /** Plugin object that defines hook implementations. */
  // eslint-disable-next-line no-var
  var plugin: ShishoPlugin;
}
