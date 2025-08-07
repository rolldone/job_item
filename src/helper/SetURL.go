package helper

import (
	"regexp"
	"strings"
)

// ReplaceKeysInURL replaces all occurrences of keys in the urlString with their corresponding values from the array of maps.
func ReplaceKeysInURL(urlString string, array []map[string]string) string {
	for _, m := range array {
		for key, value := range m {
			if strings.Contains(urlString, key) {
				re := regexp.MustCompile(regexp.QuoteMeta(key))
				urlString = re.ReplaceAllString(urlString, value)
			}
		}
	}
	return urlString
}
