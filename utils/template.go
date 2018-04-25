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
	"errors"
	"reflect"
	"strings"
	"text/template"
)

func replace(input interface{}, from string, to string) (string, error) {
	if input == nil {
		return "", errors.New("input does not exist, cannot replace")
	}
	return strings.Replace(input.(string), from, to, -1), nil
}

func split(input interface{}, delimiter string) ([]string, error) {
	return strings.Split(input.(string), delimiter), nil
}

func escape(input interface{}) string {
	return Escape(input.(string))
}

// isSet indicate is a field is defined in the template data
func isSet(v interface{}, name string) bool {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return false
	}
	return rv.FieldByName(name).IsValid()
}

// TmplFuncMap expose custom go template functions
var TmplFuncMap = template.FuncMap{
	"replace": replace,
	"split":   split,
	"escape":  escape,
	"isSet":   isSet,
}
