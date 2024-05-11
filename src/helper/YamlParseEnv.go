package helper

import (
	"os"
	"reflect"
	"regexp"

	"github.com/joho/godotenv"
)

var reVar = regexp.MustCompile(`^\${(\w+)}$`)

func Fromenv(v interface{}) {
	godotenv.Load()
	_fromenv(reflect.ValueOf(v).Elem()) // assumes pointer to struct
}

// recursive
func _fromenv(rv reflect.Value) {
	for i := 0; i < rv.NumField(); i++ {
		fv := rv.Field(i)
		if fv.Kind() == reflect.Ptr {
			fv = fv.Elem()
		}
		if fv.Kind() == reflect.Struct {
			_fromenv(fv)
			continue
		}
		if fv.Kind() == reflect.String {
			match := reVar.FindStringSubmatch(fv.String())
			if len(match) > 1 {
				fv.SetString(os.Getenv(match[1]))
			}
		}
	}
}
