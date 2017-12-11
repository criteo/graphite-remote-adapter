package utils

import (
	"fmt"
	"strings"
)

// CheckOverflow enforce m to be empty. Usefull to detect unknown fields.
func CheckOverflow(m map[string]interface{}, ctx string) error {
	if len(m) > 0 {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		return fmt.Errorf("unknown fields in %s: %s", ctx, strings.Join(keys, ", "))
	}
	return nil
}
