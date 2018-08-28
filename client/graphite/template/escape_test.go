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

package template

import (
	"testing"
)

func TestEscape(t *testing.T) {
	// Can we correctly keep and escape valid chars.
	value := "abzABZ019(){},'\"\\"
	expected := "abzABZ019\\(\\)\\{\\}\\,\\'\\\"\\\\"
	actual := Escape(value)
	if expected != actual {
		t.Errorf("Expected %s, got %s", expected, actual)
	}

	// Test percent-encoding.
	value = "é/|_;:%."
	expected = "%C3%A9%2F|_;:%25%2E"
	actual = Escape(value)
	if expected != actual {
		t.Errorf("Expected %s, got %s", expected, actual)
	}
}

func TestUnescape(t *testing.T) {
	// Can we correctly keep and unescape valid chars.
	value := "abzABZ019\\(\\)\\{\\}\\,\\'\\\"\\\\"
	expected := "abzABZ019(){},'\"\\"
	actual := Unescape(value)
	if expected != actual {
		t.Errorf("Expected %s, got %s", expected, actual)
	}

	// Test percent-decoding.
	value = "%C3%A9%2F|_;:%25%2E"
	expected = "é/|_;:%."
	actual = Unescape(value)
	if expected != actual {
		t.Errorf("Expected %s, got %s", expected, actual)
	}
}
