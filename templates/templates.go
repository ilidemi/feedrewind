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

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

//go:embed *
var templateFS embed.FS
var templatesByName map[string]*template.Template

func init() {
	caser := cases.Title(language.AmericanEnglish)
	funcMap := template.FuncMap{
		"static": util.StaticHashedPath,
		"title": func(dayOfWeek util.DayOfWeek) string {
			return caser.String(string(dayOfWeek))
		},
	}

	type NamedTemplate struct {
		DirName string
		Name    string
		Content string
	}
	allTemplates := make(map[string][]NamedTemplate)
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
			allTemplates[dirName] = append(allTemplates[dir.Name()], NamedTemplate{
				DirName: dirName,
				Name:    templateName,
				Content: string(content),
			})
		}
	}

	var sharedTemplates []NamedTemplate
	var routeTemplates []NamedTemplate
	for dirName, templates := range allTemplates {
		for _, tmpl := range templates {
			nt := NamedTemplate{
				DirName: dirName,
				Name:    path.Join(dirName, tmpl.Name),
				Content: tmpl.Content,
			}
			if dirName == "layouts" || strings.HasPrefix(tmpl.Name, "partial_") {
				sharedTemplates = append(sharedTemplates, nt)
			}
			if dirName != "layouts" {
				routeTemplates = append(routeTemplates, nt)
			}
		}
	}

	templatesByName = make(map[string]*template.Template)
	for _, routeTmpl := range routeTemplates {
		tmpl := template.Must(template.New(routeTmpl.Name).Funcs(funcMap).Parse(routeTmpl.Content))
		for _, sharedTmpl := range sharedTemplates {
			if sharedTmpl.Name == routeTmpl.Name {
				continue
			}
			template.Must(tmpl.New(sharedTmpl.Name).Parse(sharedTmpl.Content))
		}
		for _, localTmpl := range allTemplates[routeTmpl.DirName] {
			if !strings.HasPrefix(localTmpl.Name, "partial_") {
				continue
			}
			if localTmpl.Name == routeTmpl.Name {
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
// Data must have a field Session *util.Session unless we're rendering a partial
func MustWrite(w http.ResponseWriter, templateName string, data any) {
	tmpl, ok := templatesByName[templateName]
	if !ok {
		panic(fmt.Errorf("Template not found: %s", templateName))
	}

	partialIndex := strings.LastIndex(templateName, "/partial")
	isPartial := partialIndex != -1 && partialIndex == strings.LastIndex(templateName, "/")
	is404 := strings.HasSuffix(templateName, "/404")
	is500 := strings.HasSuffix(templateName, "/500")
	expectSession := !(isPartial || is404 || is500)

	if expectSession {
		sessionField := reflect.ValueOf(data).FieldByName("Session")
		if sessionField == (reflect.Value{}) ||
			sessionField.Type() != reflect.TypeOf((*util.Session)(nil)) {
			panic("Data is expected to have a field Session of type *util.Session")
		}
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
