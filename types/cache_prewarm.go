package types

import (
	"encoding/json"
	"math/big"
	"reflect"
)

// IsCachePrewarm reports an explicit max_output_tokens: 0 provider option.
// It does not infer prewarming from an absent option or a zero-valued typed
// MaxTokens setting. Route support is validated by provider adapters.
func IsCachePrewarm(request any) bool {
	if request == nil {
		return false
	}
	requestValue := reflect.ValueOf(request)
	if requestValue.Kind() == reflect.Pointer && requestValue.IsNil() {
		return false
	}
	optionsRequest, ok := request.(interface{ GetProviderOptions() map[string]any })
	if !ok {
		return false
	}
	value, present := optionsRequest.GetProviderOptions()["max_output_tokens"]
	if !present || value == nil {
		return false
	}
	if number, ok := value.(json.Number); ok {
		parsed, valid := new(big.Rat).SetString(string(number))
		return valid && parsed.Sign() == 0
	}
	number := reflect.ValueOf(value)
	switch number.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return number.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return number.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return number.Float() == 0
	default:
		return false
	}
}
