package tmpl

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path/filepath"

	"github.com/grodier/rss-go/internal/ui"
)

type Template struct {
	templateMap map[string]*template.Template
}

func parse() (map[string]*template.Template, error) {
	templateMap := map[string]*template.Template{}
	pageTmpls := "html/pages/*.tmpl.html"

	pages, err := fs.Glob(ui.Files, pageTmpls)
	if err != nil {
		return nil, err
	}

	for _, page := range pages {
		name := filepath.Base(page)

		patterns := []string{
			"html/base.tmpl.html",
			"html/partials/*.tmpl.html",
			page,
		}

		ts, err := template.New(name).ParseFS(ui.Files, patterns...)
		if err != nil {
			return nil, err
		}

		templateMap[name] = ts
	}

	return templateMap, nil
}

func NewTmpl() *Template {
	templateMap, err := parse()
	if err != nil {
		panic(err)
	}

	t := Template{
		templateMap: templateMap,
	}

	return &t
}

func (t *Template) Render(w io.Writer, page string, data any) error {
	tmpl, ok := t.templateMap[page]
	if !ok {
		return fmt.Errorf("the template %s does not exist", page)
	}
	return tmpl.ExecuteTemplate(w, "base", data)
}
