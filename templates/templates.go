package templates

import (
	"embed"
	"feedrewind/helpers"
	"html/template"
)

//go:embed *
var templateFS embed.FS
var Templates *template.Template

func init() {
	funcMap := template.FuncMap{
		"static": helpers.StaticHashedPath,
	}
	var err error
	Templates, err = template.New("index.gohtml").Funcs(funcMap).ParseFS(templateFS, "*/*.gohtml")
	if err != nil {
		panic(err)
	}
}
