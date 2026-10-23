package render

import (
	"bytes"
	"embed"
	"encoding/json"
	"html"
	"strings"

	"github.com/gtindo/changes-view/internal/engine"
)

//go:embed assets/index.html assets/app.css assets/app.js assets/vendor/cytoscape.min.js assets/vendor/elk.bundled.js assets/vendor/cytoscape-elk.js
var assets embed.FS

func asset(name string) string {
	b, err := assets.ReadFile("assets/" + name)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// script makes JavaScript safe to inline in a <script> element.
func script(js string) string {
	return strings.ReplaceAll(js, "</script", `<\/script`)
}

// HTML renders the self-contained review page.
func HTML(e *engine.Engine, version string) ([]byte, error) {
	data := build(e, version)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(data); err != nil {
		return nil, err
	}

	title := "Change review"
	if len(data.Repos) > 0 {
		var ids []string
		for _, r := range data.Repos {
			ids = append(ids, r.ID)
		}
		title += " · " + strings.Join(ids, ", ")
	}

	vendor := script(asset("vendor/cytoscape.min.js")) + "\n;\n" +
		script(asset("vendor/elk.bundled.js")) + "\n;\n" +
		script(asset("vendor/cytoscape-elk.js"))

	page := asset("index.html")
	page = strings.Replace(page, "{{TITLE}}", html.EscapeString(title), 1)
	page = strings.Replace(page, "{{CSS}}", asset("app.css"), 1)
	page = strings.Replace(page, "{{DATA}}", buf.String(), 1)
	page = strings.Replace(page, "{{VENDOR}}", vendor, 1)
	page = strings.Replace(page, "{{APP}}", script(asset("app.js")), 1)
	return []byte(page), nil
}
