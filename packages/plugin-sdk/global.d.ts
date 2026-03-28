import { ShishoPlugin } from "./hooks";
import { ShishoHostAPI } from "./host-api";

declare global {
  /** Host API object providing logging, config, HTTP, URL utilities, filesystem, archive, XML, FFmpeg, and shell access. */
  var shisho: ShishoHostAPI;
  /** Plugin object that defines hook implementations. */
  var plugin: ShishoPlugin;
}
