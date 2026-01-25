import { ShishoPlugin } from "./hooks";
import { ShishoHostAPI } from "./host-api";

declare global {
  /** Host API object providing logging, config, HTTP, filesystem, archive, XML, and FFmpeg access. */
  var shisho: ShishoHostAPI;
  /** Plugin object that defines hook implementations. */
  var plugin: ShishoPlugin;
}
