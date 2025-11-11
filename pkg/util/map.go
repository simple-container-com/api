package util

import (
	"fmt"
	"strconv"
	"strings"
)

// MapErr is similar to lo.Map, but handles error in iteratee function
func MapErr[T any, R any](collection []T, iteratee func(T, int) (R, error)) ([]R, error) {
	result := make([]R, len(collection))

	for i, item := range collection {
		res, err := iteratee(item, i)
		if err != nil {
			return nil, err
		}
		result[i] = res
	}

	return result, nil
}

type Data map[string]interface{}

// GetValue allows to extract value from the provided object by the "path" inside of its structure
// Example:
//
//			With the following object: map[string]interface{}{"a": map[string]interface{}{"b": "c"}}
//	     the value "c" can be reached by "a.b.c"
func GetValue(key string, value interface{}) (res interface{}, err error) {
	if value == nil {
		return nil, nil
	}
	overallKey := key
	if byFullKey, err := getValPart(overallKey, value, overallKey); err == nil {
		return byFullKey, nil
	}
	keys := strings.Split(key, ".")
	for _, key := range keys {
		value, err = getValPart(key, value, overallKey)
		if err != nil {
			return value, err
		}
	}
	return value, err
}

func getValPart(key string, value interface{}, overallKey string) (res interface{}, err error) {
	var (
		i  int64
		ok bool
	)
	switch value := value.(type) {
	case map[string]map[string]interface{}:
		if res, ok = value[key]; !ok { // nolint: gosimple
			err = fmt.Errorf("key not present. [key:%s] of [path:%s]", key, overallKey)
		}
	case map[string]string:
		if res, ok = value[key]; !ok { // nolint: gosimple
			err = fmt.Errorf("key not present. [key:%s] of [path:%s]", key, overallKey)
		}
	case map[string]interface{}:
		if res, ok = value[key]; !ok { // nolint: gosimple
			err = fmt.Errorf("key not present. [key:%s] of [path:%s]", key, overallKey)
		}
	case Data:
		if res, ok = value[key]; !ok { // nolint: gosimple
			err = fmt.Errorf("key not present. [key:%s] of [path:%s]", key, overallKey)
		}
	case []interface{}:
		if i, err = strconv.ParseInt(key, 10, 64); err == nil {
			array := value
			if int(i) < len(array) {
				res = array[i]
			} else {
				err = fmt.Errorf("index out of bounds. [index:%d] [array:%v] of [path:%s]", i, array, overallKey)
			}
		}
	default:
		err = fmt.Errorf("unsupported value type for key [key:%s] of [path:%s] [value:%v]", key, overallKey, value)
	}
	return res, err
}

func (data Data) AddAllIfNotExist(other Data) {
	for k, v := range other {
		if _, ok := data[k]; !ok {
			data[k] = v
		}
	}
}

func AddIfNotExist(values []string, value string) []string {
	found := false
	for _, existingValue := range values {
		if existingValue == value {
			found = true
		}
	}
	if !found {
		values = append(values, value)
	}
	return values
}

func CopyStringMap(m map[string]string) map[string]string {
	cp := make(map[string]string)
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

func CopyMap(m map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{})
	for k, v := range m {
		vm, ok := v.(map[string]interface{})
		if ok {
			cp[k] = CopyMap(vm)
		} else {
			cp[k] = v
		}
	}
	return cp
}
