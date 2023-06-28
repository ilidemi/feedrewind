package templates

import (
	"bytes"
	"embed"
	"feedrewind/util"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"reflect"
	"strings"
)

//go:embed *
var templateFS embed.FS
var templatesByName map[string]*template.Template

func init() {
	funcMap := template.FuncMap{
		"static": util.StaticHashedPath,
	}

	type namedTemplate struct {
		DirName string
		Name    string
		Content string
	}
	allTemplates := make(map[string][]namedTemplate)
	dirs, err := templateFS.ReadDir(".")
	if err != nil {
		panic(err)
	}

	const ext = ".gohtml"
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}

		dirName := dir.Name()
		files, err := templateFS.ReadDir(dirName)
		if err != nil {
			panic(err)
		}

		for _, file := range files {
			filename := file.Name()
			if !strings.HasSuffix(filename, ext) {
				continue
			}

			content, err := templateFS.ReadFile(path.Join(dirName, filename))
			if err != nil {
				panic(err)
			}

			templateName := filename[:len(filename)-len(ext)]
			allTemplates[dirName] = append(allTemplates[dir.Name()], namedTemplate{
				DirName: dirName,
				Name:    templateName,
				Content: string(content),
			})
		}
	}

	var sharedTemplates []namedTemplate
	var routeTemplates []namedTemplate
	for dirName, templates := range allTemplates {
		for _, tmpl := range templates {
			list := &routeTemplates
			if dirName == "layouts" || strings.HasPrefix(tmpl.Name, "partial_") {
				list = &sharedTemplates
			}
			*list = append(*list, namedTemplate{
				DirName: dirName,
				Name:    path.Join(dirName, tmpl.Name),
				Content: tmpl.Content,
			})
		}
	}

	templatesByName = make(map[string]*template.Template)
	for _, routeTmpl := range routeTemplates {
		tmpl := template.Must(template.New(routeTmpl.Name).Funcs(funcMap).Parse(routeTmpl.Content))
		for _, sharedTmpl := range sharedTemplates {
			template.Must(tmpl.New(sharedTmpl.Name).Parse(sharedTmpl.Content))
		}
		for _, localTmpl := range allTemplates[routeTmpl.DirName] {
			if !strings.HasPrefix(localTmpl.Name, "partial_") {
				continue
			}
			template.Must(tmpl.New(localTmpl.Name).Parse(localTmpl.Content))
		}
		templatesByName[routeTmpl.Name] = tmpl
	}
}

// Template naming conventions:
// All templates are accessible by full path without extension ("dir/template", "dir/partial_1")
// Within the same folder, partials are accessible without dir ("partial_1")
// Layout partials have to be referred by the full path, as every end user is in a different dir
// Data must have a field Session *util.Session
func MustWrite(w http.ResponseWriter, templateName string, data any) {
	tmpl, ok := templatesByName[templateName]
	if !ok {
		panic(fmt.Errorf("Template not found: %s", templateName))
	}

	sessionField := reflect.ValueOf(data).FieldByName("Session")
	if sessionField == (reflect.Value{}) ||
		sessionField.Type() != reflect.TypeOf((*util.Session)(nil)) {
		panic("Data is expected to have a field Session of type *util.Session")
	}

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, data)
	if err != nil {
		panic(err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = buf.WriteTo(w)
	if err != nil {
		panic(err)
	}
}
