package main

import (
	"fmt"
	"strings"
)

// swiftKeywords can't be property or case names without backticks.
var swiftKeywords = map[string]bool{
	"associatedtype": true, "class": true, "deinit": true, "enum": true, "extension": true, "fileprivate": true,
	"func": true, "import": true, "init": true, "inout": true, "internal": true, "let": true, "open": true,
	"operator": true, "private": true, "protocol": true, "public": true, "rethrows": true, "static": true,
	"struct": true, "subscript": true, "typealias": true, "var": true, "break": true, "case": true,
	"continue": true, "default": true, "defer": true, "do": true, "else": true, "fallthrough": true,
	"for": true, "guard": true, "if": true, "in": true, "repeat": true, "return": true, "switch": true,
	"where": true, "while": true, "as": true, "catch": true, "false": true, "is": true, "nil": true,
	"super": true, "self": true, "throw": true, "throws": true, "true": true, "try": true, "any": true,
	"some": true, "Type": true, "Self": true,
}

func swiftIdent(name string) string {
	if swiftKeywords[name] {
		return "`" + name + "`"
	}
	return name
}

// swiftModule is the Swift module the types are generated into. References
// across namespaces name it, since a namespace like Series would otherwise
// be shadowed by a type named Series in another namespace.
const swiftModule = "ShishoAPI"

// swiftName is a generated type's name as code in package from refers to
// it: bare in its own namespace, qualified by module elsewhere.
func swiftName(ref TypeRef, from string) string {
	if ref.Pkg == from {
		return ref.Name
	}
	return swiftModule + "." + pascal(ref.Pkg) + "." + ref.Name
}

// swiftType is ref's Swift type, as package from names it, without the
// Optional a pointer adds.
func swiftType(ref TypeRef, from string) string {
	var base string
	switch ref.Kind {
	case KindString:
		base = "String"
	case KindBool:
		base = "Bool"
	case KindInt:
		base = "Int"
	case KindInt32:
		base = "Int32"
	case KindDouble:
		base = "Double"
	case KindNamed, KindEnum:
		base = swiftName(ref, from)
	case KindArray:
		base = "[" + swiftType(*ref.Elem, from) + "]"
	case KindMap:
		base = "[String: " + swiftType(*ref.Elem, from) + "]"
	case KindJSON:
		base = "JSONValue"
	default:
		base = "JSONValue"
	}
	if ref.Indirect {
		return "Indirect<" + base + ">"
	}
	return base
}

// swiftZero is the value a non-optional field takes when the server leaves
// it out or sends null: Go's zero value, which is what omitempty leaves out.
func swiftZero(ref TypeRef, from string) string {
	if ref.Indirect {
		return "Indirect(" + swiftName(ref, from) + "())"
	}
	switch ref.Kind {
	case KindString:
		return `""`
	case KindBool:
		return "false"
	case KindInt, KindInt32, KindDouble:
		return "0"
	case KindNamed:
		return swiftName(ref, from) + "()"
	case KindEnum:
		return `.unknown("")`
	case KindArray:
		return "[]"
	case KindMap:
		return "[:]"
	case KindJSON:
		return ".null"
	default:
		return ".null"
	}
}

func swiftDoc(b *strings.Builder, doc, indent string) {
	if doc == "" {
		return
	}
	for _, line := range strings.Split(doc, "\n") {
		fmt.Fprintf(b, "%s/// %s\n", indent, strings.TrimRight(line, " "))
	}
}

// generateSwift writes one Swift file per package, plus the support file.
func generateSwift(m *Model) map[string]string {
	files := map[string]string{"Support.swift": swiftSupport}
	for _, pkg := range m.Packages {
		if len(pkg.Structs)+len(pkg.Enums)+len(pkg.Aliases) == 0 {
			continue
		}
		ns := pascal(pkg.Name)
		var b strings.Builder
		b.WriteString(header("tools/apptypes"))
		fmt.Fprintf(&b, "\nimport Foundation\n\n/// The API types of the Go package %s.\npublic enum %s {}\n", pkg.Name, ns)
		for _, enum := range pkg.Enums {
			fmt.Fprintf(&b, "\nextension %s {\n", ns)
			swiftEnum(&b, enum)
			b.WriteString("}\n")
		}
		for _, alias := range pkg.Aliases {
			fmt.Fprintf(&b, "\nextension %s {\n", ns)
			swiftDoc(&b, alias.Doc, "")
			t := swiftType(alias.Target, pkg.Name)
			if alias.Target.Optional {
				t += "?"
			}
			fmt.Fprintf(&b, "public typealias %s = %s\n}\n", alias.Name, t)
		}
		for _, s := range pkg.Structs {
			fmt.Fprintf(&b, "\nextension %s {\n", ns)
			swiftStruct(&b, s, pkg.Name)
			b.WriteString("}\n")
		}
		files[ns+".swift"] = b.String()
	}
	return files
}

func swiftEnum(b *strings.Builder, e *Enum) {
	swiftDoc(b, e.Doc, "")
	names := caseNames(e.Values, "unknown")
	fmt.Fprintf(b, "public enum %s: Codable, Hashable, Sendable, CaseIterable {\n", e.Name)
	for i, name := range names {
		fmt.Fprintf(b, "    case %s\n", swiftIdent(name))
		_ = i
	}
	b.WriteString("    /// A value this version of the app doesn't know, kept as sent.\n")
	b.WriteString("    case unknown(String)\n\n")
	fmt.Fprintf(b, "    public static let allCases: [%s] = [", e.Name)
	for i, name := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("." + swiftIdent(name))
	}
	b.WriteString("]\n\n")
	b.WriteString("    public init(rawValue: String) {\n        switch rawValue {\n")
	for i, v := range e.Values {
		fmt.Fprintf(b, "        case %q: self = .%s\n", v, swiftIdent(names[i]))
	}
	b.WriteString("        default: self = .unknown(rawValue)\n        }\n    }\n\n")
	b.WriteString("    public var rawValue: String {\n        switch self {\n")
	for i, v := range e.Values {
		fmt.Fprintf(b, "        case .%s: return %q\n", swiftIdent(names[i]), v)
	}
	b.WriteString("        case .unknown(let value): return value\n        }\n    }\n\n")
	b.WriteString("    public init(from decoder: Decoder) throws {\n")
	b.WriteString("        self.init(rawValue: try decoder.singleValueContainer().decode(String.self))\n    }\n\n")
	b.WriteString("    public func encode(to encoder: Encoder) throws {\n")
	b.WriteString("        var container = encoder.singleValueContainer()\n        try container.encode(rawValue)\n    }\n}\n")
}

func swiftStruct(b *strings.Builder, s *Struct, from string) {
	swiftDoc(b, s.Doc, "")
	fmt.Fprintf(b, "public struct %s: Codable, Hashable, Sendable {\n", s.Name)
	props := make([]string, len(s.Fields))
	for i, f := range s.Fields {
		props[i] = swiftIdent(camel(f.JSONName))
		t := swiftType(f.Type, from)
		if f.Type.Optional {
			t += "?"
		}
		fmt.Fprintf(b, "    public var %s: %s\n", props[i], t)
	}
	if len(s.Fields) > 0 {
		b.WriteString("\n")
	}

	// A memberwise init whose every argument has the zero value, so any
	// struct can be made empty.
	b.WriteString("    public init(")
	for i, f := range s.Fields {
		if i > 0 {
			b.WriteString(", ")
		}
		t := swiftType(f.Type, from)
		zero := swiftZero(f.Type, from)
		if f.Type.Optional {
			t += "?"
			zero = "nil"
		}
		fmt.Fprintf(b, "%s: %s = %s", props[i], t, zero)
	}
	b.WriteString(") {\n")
	for _, p := range props {
		fmt.Fprintf(b, "        self.%s = %s\n", strings.Trim(p, "`"), p)
	}
	b.WriteString("    }\n")
	if len(s.Fields) == 0 {
		b.WriteString("\n    public init(from decoder: Decoder) throws {}\n")
		b.WriteString("    public func encode(to encoder: Encoder) throws {}\n}\n")
		return
	}

	b.WriteString("\n    enum CodingKeys: String, CodingKey {\n")
	for i, f := range s.Fields {
		fmt.Fprintf(b, "        case %s = %q\n", props[i], f.JSONName)
	}
	b.WriteString("    }\n\n")

	// Decoding is lenient (ADR 0021): unknown keys are ignored, and a
	// field left out or sent as null takes its zero value.
	b.WriteString("    public init(from decoder: Decoder) throws {\n")
	b.WriteString("        let c = try decoder.container(keyedBy: CodingKeys.self)\n")
	for i, f := range s.Fields {
		t := swiftType(f.Type, from)
		if f.Type.Optional {
			fmt.Fprintf(b, "        self.%s = try c.decodeIfPresent(%s.self, forKey: .%s)\n", strings.Trim(props[i], "`"), t, props[i])
		} else {
			fmt.Fprintf(b, "        self.%s = try c.decodeIfPresent(%s.self, forKey: .%s) ?? %s\n", strings.Trim(props[i], "`"), t, props[i], swiftZero(f.Type, from))
		}
	}
	b.WriteString("    }\n}\n")
}

const swiftSupport = `// Code generated by tools/apptypes from the Go API types (ADR 0021). DO NOT EDIT.

import Foundation

/// Any JSON value, for fields the Go types leave open.
public enum JSONValue: Codable, Hashable, Sendable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([JSONValue])
    case object([String: JSONValue])

    public init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() {
            self = .null
        } else if let v = try? c.decode(Bool.self) {
            self = .bool(v)
        } else if let v = try? c.decode(Double.self) {
            self = .number(v)
        } else if let v = try? c.decode(String.self) {
            self = .string(v)
        } else if let v = try? c.decode([JSONValue].self) {
            self = .array(v)
        } else {
            self = .object(try c.decode([String: JSONValue].self))
        }
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .null: try c.encodeNil()
        case .bool(let v): try c.encode(v)
        case .number(let v): try c.encode(v)
        case .string(let v): try c.encode(v)
        case .array(let v): try c.encode(v)
        case .object(let v): try c.encode(v)
        }
    }
}

/// Boxes a struct that would otherwise contain itself.
public final class Indirect<Value: Codable & Hashable & Sendable>: Codable, Hashable, @unchecked Sendable {
    public let value: Value

    public init(_ value: Value) {
        self.value = value
    }

    public required init(from decoder: Decoder) throws {
        value = try Value(from: decoder)
    }

    public func encode(to encoder: Encoder) throws {
        try value.encode(to: encoder)
    }

    public static func == (lhs: Indirect, rhs: Indirect) -> Bool {
        lhs.value == rhs.value
    }

    public func hash(into hasher: inout Hasher) {
        hasher.combine(value)
    }
}
`
