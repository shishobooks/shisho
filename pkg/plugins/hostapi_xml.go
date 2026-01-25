package plugins

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/pkg/errors"
)

var errNoRootElement = errors.New("no root element found")

// xmlElement represents a parsed XML element in our tree structure.
type xmlElement struct {
	Tag        string
	Namespace  string
	Attributes map[string]string
	Text       string
	Children   []*xmlElement
}

// injectXMLNamespace sets up shisho.xml with parse, querySelector, and querySelectorAll.
// No file access needed - operates on in-memory strings.
func injectXMLNamespace(vm *goja.Runtime, shishoObj *goja.Object) error {
	xmlObj := vm.NewObject()
	if err := shishoObj.Set("xml", xmlObj); err != nil {
		return fmt.Errorf("failed to set shisho.xml: %w", err)
	}

	xmlObj.Set("parse", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.xml.parse: content argument is required"))
		}
		content := call.Argument(0).String()

		root, err := parseXML(content)
		if err != nil {
			panic(vm.ToValue("shisho.xml.parse: " + err.Error()))
		}

		return elementToGojaValue(vm, root)
	})

	xmlObj.Set("querySelector", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.xml.querySelector: doc and selector arguments are required"))
		}

		doc := gojaValueToElement(vm, call.Argument(0))
		if doc == nil {
			panic(vm.ToValue("shisho.xml.querySelector: invalid document"))
		}
		selector := call.Argument(1).String()

		// Parse optional namespaces argument
		var namespaces map[string]string
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
			namespaces = extractNamespacesMap(vm, call.Argument(2))
		}

		match := querySelector(doc, selector, namespaces)
		if match == nil {
			return goja.Null()
		}
		return elementToGojaValue(vm, match)
	})

	xmlObj.Set("querySelectorAll", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.xml.querySelectorAll: doc and selector arguments are required"))
		}

		doc := gojaValueToElement(vm, call.Argument(0))
		if doc == nil {
			panic(vm.ToValue("shisho.xml.querySelectorAll: invalid document"))
		}
		selector := call.Argument(1).String()

		// Parse optional namespaces argument
		var namespaces map[string]string
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
			namespaces = extractNamespacesMap(vm, call.Argument(2))
		}

		matches := querySelectorAll(doc, selector, namespaces)
		result := make([]interface{}, len(matches))
		for i, m := range matches {
			result[i] = elementToGojaValue(vm, m)
		}
		return vm.ToValue(result)
	})

	return nil
}

// parseXML parses an XML string into our tree structure.
func parseXML(content string) (*xmlElement, error) {
	decoder := xml.NewDecoder(strings.NewReader(content))
	var root *xmlElement
	var stack []*xmlElement

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, fmt.Errorf("XML parse error: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			elem := &xmlElement{
				Tag:        t.Name.Local,
				Namespace:  t.Name.Space,
				Attributes: make(map[string]string),
				Children:   make([]*xmlElement, 0),
			}
			for _, attr := range t.Attr {
				attrName := attr.Name.Local
				if attr.Name.Space != "" {
					attrName = attr.Name.Space + ":" + attr.Name.Local
				}
				elem.Attributes[attrName] = attr.Value
			}

			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, elem)
			} else {
				root = elem
			}
			stack = append(stack, elem)

		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

		case xml.CharData:
			text := string(t)
			if len(stack) > 0 {
				current := stack[len(stack)-1]
				current.Text += text
			}
		}
	}

	if root == nil {
		return nil, errNoRootElement
	}

	return root, nil
}

// elementToGojaValue converts our xmlElement to a goja object.
func elementToGojaValue(vm *goja.Runtime, elem *xmlElement) goja.Value {
	obj := vm.NewObject()
	obj.Set("tag", elem.Tag)             //nolint:errcheck
	obj.Set("namespace", elem.Namespace) //nolint:errcheck
	obj.Set("text", elem.Text)           //nolint:errcheck

	attrs := vm.NewObject()
	for k, v := range elem.Attributes {
		attrs.Set(k, v) //nolint:errcheck
	}
	obj.Set("attributes", attrs) //nolint:errcheck

	children := make([]interface{}, len(elem.Children))
	for i, child := range elem.Children {
		children[i] = elementToGojaValue(vm, child)
	}
	obj.Set("children", vm.ToValue(children)) //nolint:errcheck

	return obj
}

// gojaValueToElement converts a goja object back to our xmlElement for querying.
func gojaValueToElement(vm *goja.Runtime, val goja.Value) *xmlElement {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}

	obj := val.ToObject(vm)
	if obj == nil {
		return nil
	}

	elem := &xmlElement{
		Attributes: make(map[string]string),
		Children:   make([]*xmlElement, 0),
	}

	if tag := obj.Get("tag"); tag != nil && !goja.IsUndefined(tag) {
		elem.Tag = tag.String()
	}
	if ns := obj.Get("namespace"); ns != nil && !goja.IsUndefined(ns) {
		elem.Namespace = ns.String()
	}
	if text := obj.Get("text"); text != nil && !goja.IsUndefined(text) {
		elem.Text = text.String()
	}

	if attrs := obj.Get("attributes"); attrs != nil && !goja.IsUndefined(attrs) {
		attrsObj := attrs.ToObject(vm)
		for _, key := range attrsObj.Keys() {
			elem.Attributes[key] = attrsObj.Get(key).String()
		}
	}

	if children := obj.Get("children"); children != nil && !goja.IsUndefined(children) {
		childrenObj := children.ToObject(vm)
		// Get length property
		lengthVal := childrenObj.Get("length")
		if lengthVal != nil && !goja.IsUndefined(lengthVal) {
			length := int(lengthVal.ToInteger())
			for i := 0; i < length; i++ {
				childVal := childrenObj.Get(strconv.Itoa(i))
				if child := gojaValueToElement(vm, childVal); child != nil {
					elem.Children = append(elem.Children, child)
				}
			}
		}
	}

	return elem
}

// extractNamespacesMap reads a goja object as a map of prefix -> URI.
func extractNamespacesMap(vm *goja.Runtime, val goja.Value) map[string]string {
	obj := val.ToObject(vm)
	result := make(map[string]string)
	for _, key := range obj.Keys() {
		result[key] = obj.Get(key).String()
	}
	return result
}

// parseSelector parses a CSS namespace selector like "prefix|tagName" or just "tagName".
// Returns the namespace URI (resolved from the namespaces map) and the local tag name.
func parseSelector(selector string, namespaces map[string]string) (nsURI string, tagName string, hasNS bool) {
	if idx := strings.Index(selector, "|"); idx >= 0 {
		prefix := selector[:idx]
		tagName = selector[idx+1:]
		if namespaces != nil {
			nsURI = namespaces[prefix]
		}
		return nsURI, tagName, true
	}
	return "", selector, false
}

// matchesSelector checks if an element matches the given parsed selector.
func matchesSelector(elem *xmlElement, nsURI, tagName string, hasNS bool) bool {
	if elem.Tag != tagName {
		return false
	}
	if hasNS {
		return elem.Namespace == nsURI
	}
	// No namespace specified: match any element with the given local name
	return true
}

// querySelector finds the first element matching the selector (DFS).
func querySelector(root *xmlElement, selector string, namespaces map[string]string) *xmlElement {
	nsURI, tagName, hasNS := parseSelector(selector, namespaces)

	// Check root itself
	if matchesSelector(root, nsURI, tagName, hasNS) {
		return root
	}

	// DFS through children
	for _, child := range root.Children {
		if result := querySelector(child, selector, namespaces); result != nil {
			return result
		}
	}

	return nil
}

// querySelectorAll finds all elements matching the selector (DFS).
func querySelectorAll(root *xmlElement, selector string, namespaces map[string]string) []*xmlElement {
	nsURI, tagName, hasNS := parseSelector(selector, namespaces)
	var results []*xmlElement
	querySelectorAllRecurse(root, nsURI, tagName, hasNS, &results)
	return results
}

// querySelectorAllRecurse recursively searches for matching elements.
func querySelectorAllRecurse(elem *xmlElement, nsURI, tagName string, hasNS bool, results *[]*xmlElement) {
	if matchesSelector(elem, nsURI, tagName, hasNS) {
		*results = append(*results, elem)
	}
	for _, child := range elem.Children {
		querySelectorAllRecurse(child, nsURI, tagName, hasNS, results)
	}
}
