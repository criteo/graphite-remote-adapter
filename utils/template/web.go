package template

import (
	"bytes"
	"fmt"
	"path/filepath"
	"text/template"

	"github.com/criteo/graphite-remote-adapter/ui"
)

func getTemplate(name string) (string, error) {
	baseTmpl, err := ui.Asset("templates/_base.html")
	if err != nil {
		return "", fmt.Errorf("error reading base template: %s", err)
	}
	pageTmpl, err := ui.Asset(filepath.Join("templates", name))
	if err != nil {
		return "", fmt.Errorf("error reading page template %s: %s", name, err)
	}
	return string(baseTmpl) + string(pageTmpl), nil
}

// ExecuteTemplate renders template for given name with provided data.
func ExecuteTemplate(name string, data interface{}) ([]byte, error) {
	text, err := getTemplate(name)
	if err != nil {
		return nil, err
	}

	tmpl := template.New(name).Funcs(template.FuncMap(TmplFuncMap))
	tmpl, err = tmpl.Parse(text)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	err = tmpl.Execute(&buffer, data)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
