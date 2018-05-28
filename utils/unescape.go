// Copyright 2015 The Prometheus Authors
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
	"bytes"
	"net/url"
	"strings"
)

// Unescape takes string that have been escape to comply graphite's storage format
// and return their original value
func Unescape(tv string) string {
	// unescape percent encoding
	new, _ := url.PathUnescape(tv)
	length := len(new)
	result := bytes.NewBuffer(make([]byte, 0, length))
	for i := 0; i < length; i++ {
		b := new[i]
		switch {
		// / could have been double escaped
		case b == '\\' && i < length-1:
			n := new[i+1]
			// if next byte is not a symbol, this / is legitimate
			if strings.IndexByte(symbols, n) == -1 {
				result.WriteByte(b)
			}
		default:
			result.WriteByte(b)
		}
	}
	return result.String()
}
