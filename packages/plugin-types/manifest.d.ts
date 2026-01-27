/** Input converter capability declaration. */
export interface InputConverterCap {
  description?: string;
  /** File extensions this converter accepts (e.g., ["mobi"]). */
  sourceTypes: string[];
  /** MIME types this converter accepts. */
  mimeTypes?: string[];
  /** Target file type to convert to (e.g., "epub"). */
  targetType: string;
}

/** File parser capability declaration. */
export interface FileParserCap {
  description?: string;
  /** File extensions this parser handles (e.g., ["pdf"]). */
  types: string[];
  /** MIME types this parser handles (e.g., ["application/pdf"]). */
  mimeTypes?: string[];
}

/** Output generator capability declaration. */
export interface OutputGeneratorCap {
  description?: string;
  /** Unique format identifier (e.g., "mobi"). */
  id: string;
  /** Display name (e.g., "MOBI"). */
  name: string;
  /** Source file types this generator can convert from. */
  sourceTypes: string[];
}

/** Metadata enricher capability declaration. */
export interface MetadataEnricherCap {
  description?: string;
  /** File types this enricher applies to (e.g., ["epub", "cbz"]). */
  fileTypes?: string[];
}

/** Custom identifier type declaration. */
export interface IdentifierTypeCap {
  /** Unique identifier type ID (e.g., "goodreads"). */
  id: string;
  /** Display name (e.g., "Goodreads"). */
  name: string;
  /** URL template with {value} placeholder. */
  urlTemplate?: string;
  /** Regex pattern for validation. */
  pattern?: string;
}

/** HTTP access capability declaration. */
export interface HTTPAccessCap {
  description?: string;
  /** Allowed domains for HTTP requests. */
  domains: string[];
}

/** File access capability declaration. */
export interface FileAccessCap {
  /** Access level: "read" or "readwrite". */
  level: "read" | "readwrite";
  description?: string;
}

/** FFmpeg access capability declaration. */
export interface FFmpegAccessCap {
  description?: string;
}

/** Shell access capability declaration. */
export interface ShellAccessCap {
  description?: string;
  /** Allowed commands (e.g., ["convert", "magick", "identify"]). */
  commands: string[];
}

/** All plugin capabilities. */
export interface Capabilities {
  inputConverter?: InputConverterCap;
  fileParser?: FileParserCap;
  outputGenerator?: OutputGeneratorCap;
  metadataEnricher?: MetadataEnricherCap;
  identifierTypes?: IdentifierTypeCap[];
  httpAccess?: HTTPAccessCap;
  fileAccess?: FileAccessCap;
  ffmpegAccess?: FFmpegAccessCap;
  shellAccess?: ShellAccessCap;
}

/** Option for select-type config fields. */
export interface SelectOption {
  value: string;
  label: string;
}

/** A single configuration field definition. */
export interface ConfigField {
  /** Field type. */
  type: "string" | "boolean" | "number" | "select" | "textarea";
  /** Display label. */
  label: string;
  /** Help text description. */
  description?: string;
  /** Whether this field is required. */
  required?: boolean;
  /** Whether this field contains sensitive data. */
  secret?: boolean;
  /** Default value. */
  default?: string | number | boolean;
  /** Minimum value (number fields). */
  min?: number;
  /** Maximum value (number fields). */
  max?: number;
  /** Options (select fields). */
  options?: SelectOption[];
}

/** Configuration schema mapping field keys to definitions. */
export interface ConfigSchema {
  [key: string]: ConfigField;
}

/** Complete plugin manifest (manifest.json). */
export interface PluginManifest {
  /** Must be 1. */
  manifestVersion: 1;
  /** Plugin identifier (e.g., "goodreads-metadata"). */
  id: string;
  /** Display name. */
  name: string;
  /** Semver version string. */
  version: string;
  description?: string;
  author?: string;
  homepage?: string;
  license?: string;
  /** Minimum Shisho version required. */
  minShishoVersion?: string;
  capabilities?: Capabilities;
  configSchema?: ConfigSchema;
}
