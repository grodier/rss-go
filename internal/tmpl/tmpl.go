package tmpl

import (
	"html/template"
	"io/fs"

	"github.com/grodier/rss-go/internal/ui"
)

func parse() (*template.Template, error) {
	pageTmpls := "html/pages/*.tmpl.html"

	files, err := fs.Glob(ui.Files, pageTmpls)
	if err != nil {
		return nil, err
	}

	return template.New("root").ParseFS(ui.Files, files...)
}

func NewTmpl() *template.Template {
	t, err := parse()
	if err != nil {
		panic(err)
	}
	return t
}
