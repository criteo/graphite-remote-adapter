// Copyright 2017 Thibault Chataigner <thibault.chataigner@gmail.com>
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"strings"
	"text/template"
)

func replace(input interface{}, from string, to string) string {
	return strings.Replace(input.(string), from, to, -1)
}

func split(input interface{}, delimiter string) ([]string, error) {
	return strings.Split(input.(string), delimiter), nil
}

func escape(input interface{}) string {
	return Escape(input.(string))
}

// TmplFuncMap expose custom go template functions
var TmplFuncMap = template.FuncMap{
	"replace": replace,
	"split":   split,
	"escape":  escape,
}
