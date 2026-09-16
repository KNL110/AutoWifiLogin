package portal

import (
	"io"

	"golang.org/x/net/html"
)

// parseInputFields walks an HTML document and collects name/value pairs
// from every <input> element it finds. This is deliberately generic (not
// tied to specific field names) so small portal page changes don't break
// login — only a genuinely new required field would.
func parseInputFields(r io.Reader) (map[string]string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	fields := make(map[string]string)
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "input" {
			var name, value string
			for _, a := range n.Attr {
				switch a.Key {
				case "name":
					name = a.Val
				case "value":
					value = a.Val
				}
			}
			if name != "" {
				fields[name] = value
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return fields, nil
}
