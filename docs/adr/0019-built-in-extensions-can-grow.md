# The built-in extension set can grow, and plugin parsers yield to it

Plugin file parsers cannot claim an extension Shisho parses itself, as ADR 0009 also applied to MOBI and AZW3. The plugin docs listed those extensions as a fixed set, so by the breaking-change rule each new built-in extension changed the plugin contract, though MOBI and AZW3 shipped without the marker. The contract now says the set can grow in any release: once Shisho parses an extension itself, a plugin file parser that declares it is no longer used for it, and the scan job log names each plugin file parser skipped this way. Adding a built-in extension is therefore not a breaking change. The release that introduces this rule (with `.m4a`, ADR 0018) is marked breaking for it once.

## Considered options

- **Plugin parsers win over built-in ones.** Would let a user replace Shisho's parser with their own. Rejected for now: flipping precedence would suddenly activate every plugin that already declares `epub` or `m4b` and is ignored today, and the reader, Generated Files, and delivery all assume the built-in parser read the file. A user who adopts first-party support for a type usually wants it over a plugin's.
- **Grandfathering.** A plugin keeps an extension it claimed before Shisho added it. Rejected as precise but costly: Shisho would have to remember when each claim was made, for a case that comes up rarely.

## Consequences

Input converters are unaffected. They still run on their source types whether or not Shisho parses them, which ADR 0009 relies on for converters that decrypt a file into a sibling copy. A converter that turned a newly built-in type into another one, such as `.m4a` into `.m4b`, now leaves both files as main Files of the Book. Its upgrade note tells operators to remove the converter and the copies it made.
