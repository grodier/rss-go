package tmpl

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"

	"github.com/grodier/rss-go/internal/ui"
)

type Template struct {
	templateMap map[string]*template.Template
}

func parse() (map[string]*template.Template, error) {
	templateMap := map[string]*template.Template{}
	pageTmpls := "html/pages/*.tmpl.html"

	files, err := fs.Glob(ui.Files, pageTmpls)
	if err != nil {
		return nil, err
	}

	tmplate, err := template.New("root").ParseFS(ui.Files, files...)
	if err != nil {
		return nil, err
	}

	templateMap["root"] = tmplate

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
	tmpl, ok := t.templateMap["root"]
	if !ok {
		return fmt.Errorf("the template %s does not exist", page)
	}
	return tmpl.ExecuteTemplate(w, page, data)
}
