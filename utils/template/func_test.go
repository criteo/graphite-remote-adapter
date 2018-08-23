package template

import (
	"bytes"
	"testing"
	"text/template"
)

var hostWithPort = "hostbase-otherpart1234:4567"
var hostWithNumbers = "long-hostname-machine01234"

func Test_aTemplateCanSplitStringArray(t *testing.T) {

	tmpl, err := template.New("test").Funcs(TmplFuncMap).Parse(`{{ index ( split . ":" ) 0}}`)
	if err != nil {
		t.Errorf("error parsing template: %v", err)
	}

	buf := bytes.NewBufferString("")
	if err = tmpl.Execute(buf, hostWithPort); err != nil {
		t.Errorf("error executing template: %v", err)
	}

	if string(buf.Bytes()) != "hostbase-otherpart1234" {
		t.Errorf("split function not properly implemented or template misconfigured")
	}
}

func Test_aTemplateCanReplaceRegex(t *testing.T) {
	tmpl, err := template.New("test").Funcs(TmplFuncMap).Parse("{{ replaceRegex . `^([a-z_\\-]*)[0-9]*$` `$1` }}")
	if err != nil {
		t.Errorf("error parsing template: %v", err)
	}

	buf := bytes.NewBufferString("")
	if err = tmpl.Execute(buf, hostWithNumbers); err != nil {
		t.Errorf("error executing template: %v", err)
	}
	actual := string(buf.Bytes())
	if actual != "long-hostname-machine" {
		t.Errorf("replaceRegex function not properly implemented or template misconfigured: result %s", actual)
	}
}
