package plugins

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/andybalholm/cascadia"
	"github.com/dop251/goja"
	"golang.org/x/net/html"
)

// htmlElement represents a parsed HTML element.
type htmlElement struct {
	Tag        string
	Attributes map[string]string
	Text       string
	InnerHTML  string
	Children   []*htmlElement
}

// injectHTMLNamespace sets up shisho.html with querySelector and querySelectorAll.
// No file access needed - operates on in-memory strings.
func injectHTMLNamespace(vm *goja.Runtime, shishoObj *goja.Object) error {
	htmlObj := vm.NewObject()
	if err := shishoObj.Set("html", htmlObj); err != nil {
		return fmt.Errorf("failed to set shisho.html: %w", err)
	}

	htmlObj.Set("querySelector", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.html.querySelector: html and selector arguments are required"))
		}
		htmlStr := call.Argument(0).String()
		selectorStr := call.Argument(1).String()

		sel, err := cascadia.Parse(selectorStr)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.html.querySelector: invalid selector: %s", err)))
		}

		doc, err := html.Parse(strings.NewReader(htmlStr))
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.html.querySelector: HTML parse error: %s", err)))
		}

		match := cascadia.Query(doc, sel)
		if match == nil {
			return goja.Null()
		}

		elem := nodeToHTMLElement(match)
		return htmlElementToGojaValue(vm, elem)
	})

	htmlObj.Set("querySelectorAll", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.html.querySelectorAll: html and selector arguments are required"))
		}
		htmlStr := call.Argument(0).String()
		selectorStr := call.Argument(1).String()

		sel, err := cascadia.Parse(selectorStr)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.html.querySelectorAll: invalid selector: %s", err)))
		}

		doc, err := html.Parse(strings.NewReader(htmlStr))
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.html.querySelectorAll: HTML parse error: %s", err)))
		}

		matches := cascadia.QueryAll(doc, sel)
		result := make([]interface{}, len(matches))
		for i, m := range matches {
			elem := nodeToHTMLElement(m)
			result[i] = htmlElementToGojaValue(vm, elem)
		}
		return vm.ToValue(result)
	})

	return nil
}

// nodeToHTMLElement converts an html.Node to our htmlElement struct.
func nodeToHTMLElement(n *html.Node) *htmlElement {
	elem := &htmlElement{
		Tag:        n.Data,
		Attributes: make(map[string]string),
		Children:   make([]*htmlElement, 0),
	}

	for _, attr := range n.Attr {
		elem.Attributes[attr.Key] = attr.Val
	}

	elem.Text = extractText(n)
	elem.InnerHTML = renderInnerHTML(n)

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			elem.Children = append(elem.Children, nodeToHTMLElement(c))
		}
	}

	return elem
}

// extractText recursively collects all text content from a node and its descendants.
func extractText(n *html.Node) string {
	var sb strings.Builder
	extractTextRecurse(n, &sb)
	return sb.String()
}

// extractTextRecurse walks the node tree collecting text nodes.
func extractTextRecurse(n *html.Node, sb *strings.Builder) {
	if n.Type == html.TextNode {
		sb.WriteString(n.Data)
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractTextRecurse(c, sb)
	}
}

// renderInnerHTML renders the inner HTML of a node (its children) as a string.
func renderInnerHTML(n *html.Node) string {
	var buf bytes.Buffer
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		html.Render(&buf, c) //nolint:errcheck
	}
	return buf.String()
}

// htmlElementToGojaValue converts an htmlElement to a goja object.
func htmlElementToGojaValue(vm *goja.Runtime, elem *htmlElement) goja.Value {
	obj := vm.NewObject()
	obj.Set("tag", elem.Tag)             //nolint:errcheck
	obj.Set("text", elem.Text)           //nolint:errcheck
	obj.Set("innerHTML", elem.InnerHTML) //nolint:errcheck

	attrs := vm.NewObject()
	for k, v := range elem.Attributes {
		attrs.Set(k, v) //nolint:errcheck
	}
	obj.Set("attributes", attrs) //nolint:errcheck

	children := make([]interface{}, len(elem.Children))
	for i, child := range elem.Children {
		children[i] = htmlElementToGojaValue(vm, child)
	}
	obj.Set("children", vm.ToValue(children)) //nolint:errcheck

	return obj
}
