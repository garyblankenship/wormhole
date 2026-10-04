package types

import (
	"encoding/json"
	"math/big"
	"reflect"
	"strconv"
)

// schemaNumber accepts the numeric representations used by decoded and generated schemas.
func schemaNumber(data any) (float64, bool) {
	if n, ok := data.(json.Number); ok {
		v, err := n.Float64()
		return v, err == nil
	}
	v := reflect.ValueOf(data)
	if !v.IsValid() {
		return 0, false
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), true
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	}
	return 0, false
}

func equalSchemaNumbers(a, b any) bool {
	x, xok := exactSchemaNumber(a)
	y, yok := exactSchemaNumber(b)
	return xok && yok && x.Cmp(y) == 0
}

func exactSchemaNumber(data any) (*big.Rat, bool) {
	var text string
	if n, ok := data.(json.Number); ok {
		text = string(n)
	} else {
		v := reflect.ValueOf(data)
		if !v.IsValid() {
			return nil, false
		}
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			text = strconv.FormatInt(v.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			text = strconv.FormatUint(v.Uint(), 10)
		case reflect.Float32, reflect.Float64:
			text = strconv.FormatFloat(v.Float(), 'g', -1, v.Type().Bits())
		default:
			return nil, false
		}
	}
	return new(big.Rat).SetString(text)
}
