package utils

import "testing"

func TestPrepareURL(t *testing.T) {
	expectedURL := "https://guest:guest@greathost:83232/my/path?q=query&toto=lulu"

	u, _ := PrepareURL(
		"https://guest:guest@greathost:83232", "/my/path",
		map[string]string{"q": "query", "toto": "lulu"},
	)
	actualURL := u.String()

	if actualURL != expectedURL {
		t.Errorf("Expected %s, got %s", expectedURL, actualURL)
	}
}
