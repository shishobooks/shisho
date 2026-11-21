package main

import (
	"fmt"
	"strings"
)

// kotlinPackage is the package holding the support code; each Go
// package's types are in a package under it.
const kotlinPackage = "com.shishobooks.shisho.api"

// kotlinName is a generated type's name as code in package from refers to
// it: bare in its own package, fully qualified elsewhere.
func kotlinName(ref TypeRef, from string) string {
	if ref.Pkg == from {
		return ref.Name
	}
	return kotlinPackage + "." + ref.Pkg + "." + ref.Name
}

// kotlinKeywords can't be names without backticks.
var kotlinKeywords = map[string]bool{
	"as": true, "break": true, "class": true, "continue": true, "do": true, "else": true, "false": true,
	"for": true, "fun": true, "if": true, "in": true, "interface": true, "is": true, "null": true,
	"object": true, "package": true, "return": true, "super": true, "this": true, "throw": true,
	"true": true, "try": true, "typealias": true, "typeof": true, "val": true, "var": true, "when": true,
	"while": true,
}

func kotlinIdent(name string) string {
	if kotlinKeywords[name] {
		return "`" + name + "`"
	}
	return name
}

// kotlinType is ref's Kotlin type, as package from names it, without the ?
// a pointer adds.
func kotlinType(ref TypeRef, from string) string {
	switch ref.Kind {
	case KindString:
		return "kotlin.String"
	case KindBool:
		return "kotlin.Boolean"
	case KindInt:
		return "kotlin.Long"
	case KindInt32:
		return "kotlin.Int"
	case KindDouble:
		return "kotlin.Double"
	case KindNamed, KindEnum:
		return kotlinName(ref, from)
	case KindArray:
		return "kotlin.collections.List<" + kotlinType(*ref.Elem, from) + ">"
	case KindMap:
		return "kotlin.collections.Map<kotlin.String, " + kotlinType(*ref.Elem, from) + ">"
	case KindJSON:
		return "JsonElement"
	default:
		return "JsonElement"
	}
}

// kotlinZero is the default a non-optional property takes when the server
// leaves it out or, through coerceInputValues, sends null: Go's zero value.
func kotlinZero(ref TypeRef, from string) string {
	switch ref.Kind {
	case KindString:
		return `""`
	case KindBool:
		return "false"
	case KindInt:
		return "0L"
	case KindInt32:
		return "0"
	case KindDouble:
		return "0.0"
	case KindNamed:
		return kotlinName(ref, from) + "()"
	case KindEnum:
		return kotlinName(ref, from) + `.Unknown("")`
	case KindArray:
		return "kotlin.collections.emptyList()"
	case KindMap:
		return "kotlin.collections.emptyMap()"
	case KindJSON:
		return "JsonNull"
	default:
		return "JsonNull"
	}
}

func kotlinDoc(b *strings.Builder, doc, indent string) {
	if doc == "" {
		return
	}
	fmt.Fprintf(b, "%s/**\n", indent)
	for _, line := range strings.Split(doc, "\n") {
		line = strings.ReplaceAll(line, "*/", "* /")
		fmt.Fprintf(b, "%s * %s\n", indent, strings.TrimRight(line, " "))
	}
	fmt.Fprintf(b, "%s */\n", indent)
}

const kotlinImports = `import kotlinx.serialization.KSerializer
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
`

// generateKotlin writes one Kotlin file per package, plus the support file.
func generateKotlin(m *Model) map[string]string {
	files := map[string]string{"Support.kt": kotlinSupport}
	for _, pkg := range m.Packages {
		if len(pkg.Structs)+len(pkg.Enums)+len(pkg.Aliases) == 0 {
			continue
		}
		var b strings.Builder
		b.WriteString(header("tools/apptypes"))
		fmt.Fprintf(&b, "@file:Suppress(\"unused\", \"RedundantVisibilityModifier\")\n\n// The API types of the Go package %s.\npackage %s.%s\n\n%s", pkg.Name, kotlinPackage, pkg.Name, kotlinImports)
		for _, enum := range pkg.Enums {
			b.WriteString("\n")
			kotlinEnum(&b, enum)
		}
		for _, alias := range pkg.Aliases {
			b.WriteString("\n")
			kotlinDoc(&b, alias.Doc, "")
			t := kotlinType(alias.Target, pkg.Name)
			if alias.Target.Optional {
				t += "?"
			}
			fmt.Fprintf(&b, "public typealias %s = %s\n", alias.Name, t)
		}
		for _, s := range pkg.Structs {
			b.WriteString("\n")
			kotlinStruct(&b, s, pkg.Name)
		}
		files[pascal(pkg.Name)+".kt"] = b.String()
	}
	return files
}

func kotlinEnum(b *strings.Builder, e *Enum) {
	kotlinDoc(b, e.Doc, "")
	names := caseNames(e.Values, "unknown", "of", "raw", "entries", "serializer")
	fmt.Fprintf(b, "@Serializable(with = %s.Serializer::class)\n", e.Name)
	fmt.Fprintf(b, "public sealed class %s(public val raw: kotlin.String) {\n", e.Name)
	for i, v := range e.Values {
		fmt.Fprintf(b, "    public data object %s : %s(%q)\n", kotlinIdent(pascal(names[i])), e.Name, v)
	}
	b.WriteString("\n    /** A value this version of the app doesn't know, kept as sent. */\n")
	fmt.Fprintf(b, "    public class Unknown(raw: kotlin.String) : %s(raw) {\n", e.Name)
	b.WriteString("        override fun equals(other: kotlin.Any?): kotlin.Boolean = other is Unknown && other.raw == raw\n")
	b.WriteString("        override fun hashCode(): kotlin.Int = raw.hashCode()\n")
	b.WriteString("        override fun toString(): kotlin.String = \"Unknown($raw)\"\n    }\n\n")
	b.WriteString("    public companion object {\n")
	fmt.Fprintf(b, "        public val entries: kotlin.collections.List<%s> = kotlin.collections.listOf(", e.Name)
	for i, name := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(kotlinIdent(pascal(name)))
	}
	b.WriteString(")\n\n")
	fmt.Fprintf(b, "        public fun of(raw: kotlin.String): %s = entries.firstOrNull { value -> value.raw == raw } ?: Unknown(raw)\n    }\n\n", e.Name)
	fmt.Fprintf(b, "    public object Serializer : KSerializer<%s> {\n", e.Name)
	fmt.Fprintf(b, "        override val descriptor: SerialDescriptor = PrimitiveSerialDescriptor(%q, PrimitiveKind.STRING)\n", kotlinPackage+"."+e.Name)
	fmt.Fprintf(b, "        override fun serialize(encoder: Encoder, value: %s): kotlin.Unit = encoder.encodeString(value.raw)\n", e.Name)
	fmt.Fprintf(b, "        override fun deserialize(decoder: Decoder): %s = of(decoder.decodeString())\n    }\n}\n", e.Name)
}

func kotlinStruct(b *strings.Builder, s *Struct, from string) {
	kotlinDoc(b, s.Doc, "")
	b.WriteString("@Serializable\n")
	if len(s.Fields) == 0 {
		fmt.Fprintf(b, "public class %s {\n", s.Name)
		b.WriteString("    override fun equals(other: kotlin.Any?): kotlin.Boolean = other is " + s.Name + "\n")
		b.WriteString("    override fun hashCode(): kotlin.Int = 0\n}\n")
		return
	}
	fmt.Fprintf(b, "public data class %s(\n", s.Name)
	for _, f := range s.Fields {
		t := kotlinType(f.Type, from)
		zero := kotlinZero(f.Type, from)
		if f.Type.Optional {
			t += "?"
			zero = "null"
		}
		fmt.Fprintf(b, "    @SerialName(%q) public val %s: %s = %s,\n", f.JSONName, kotlinIdent(camel(f.JSONName)), t, zero)
	}
	b.WriteString(")\n")
}

const kotlinSupport = `// Code generated by tools/apptypes from the Go API types (ADR 0021). DO NOT EDIT.
package ` + kotlinPackage + `

import kotlinx.serialization.json.Json

/**
 * The JSON settings every Shisho API response is decoded with. They make
 * decoding lenient (ADR 0021): unknown keys are ignored, a field left out or
 * sent as null takes its zero value, and enums keep unknown values.
 */
public val ShishoJson: Json = Json {
    ignoreUnknownKeys = true
    coerceInputValues = true
    explicitNulls = false
}
`
