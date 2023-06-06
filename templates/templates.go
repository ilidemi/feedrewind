package templates

import (
	"embed"
	"fmt"
	"html/template"
)

//go:embed *
var templateFS embed.FS
var Templates *template.Template

func init() {
	var err error
	Templates, err = template.ParseFS(templateFS, "*/*.gohtml")
	if err != nil {
		panic(err)
	}
	fmt.Println(Templates.Name())
}
