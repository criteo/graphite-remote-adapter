package utils

import "testing"

func TestTruncateString(t *testing.T) {
	var result string

	result = TruncateString("test", -1)
	if "" != result {
		t.Errorf("Expected %s, got %s", "", result)
	}

	result = TruncateString("test", 10)
	if "test" != result {
		t.Errorf("Expected %s, got %s", "test", result)
	}

	result = TruncateString("0123456789abcd...", 10)
	if "0123456789" != result {
		t.Errorf("Expected %s, got %s", "0123456789", result)
	}

	result = TruncateString("测验", 1)
	if "测" != result {
		t.Errorf("Expected %s, got %s", "测", result)
	}

}
