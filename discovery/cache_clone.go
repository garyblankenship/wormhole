package discovery

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

// cacheSchemaVersion is stamped into every persisted CacheEntry shard. Bump
// this when the on-disk CacheEntry shape changes so old shards are treated
// as a cache miss instead of being blindly unmarshaled into a new shape.
const cacheSchemaVersion = 1

// writeShardAtomic writes data to path atomically: a unique per-call temp
// file (avoiding the collision a fixed shared ".tmp" name would hit across
// concurrent processes/goroutines writing the same path), fsync'd before
// the rename so a crash can't leave a truncated shard on disk.
func writeShardAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tmp.Name()
	defer func() {
		_ = os.Remove(tempPath) // #nosec G304 - path validated via ValidatePath -- no-op once renamed
	}()

	if err := tmp.Chmod(0600); err != nil { // #nosec G304 - path validated via ValidatePath
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path) // #nosec G304 - path validated via ValidatePath
}

func baseProviderKey(provider string) string {
	base, _, found := strings.Cut(provider, "__")
	if found && base != "" {
		return base
	}
	return provider
}

func cacheLookupKeys(provider string) []string {
	base := baseProviderKey(provider)
	if base == provider {
		return []string{provider}
	}
	return []string{provider, base}
}

func cloneModels(models []*types.ModelInfo) []*types.ModelInfo {
	if models == nil {
		return nil
	}

	cloned := make([]*types.ModelInfo, len(models))
	for i, model := range models {
		if model == nil {
			continue
		}

		modelCopy := *model
		if model.Cost != nil {
			costCopy := *model.Cost
			modelCopy.Cost = &costCopy
		}
		modelCopy.Capabilities = append([]types.ModelCapability(nil), model.Capabilities...)
		modelCopy.Constraints = cloneConstraints(model.Constraints)
		cloned[i] = &modelCopy
	}
	return cloned
}

func cloneConstraints(constraints map[string]any) map[string]any {
	if constraints == nil {
		return nil
	}

	cloned := make(map[string]any, len(constraints))
	for key, value := range constraints {
		cloned[key] = cloneConstraintValue(value)
	}
	return cloned
}

func cloneConstraintValue(value any) any {
	if value == nil {
		return nil
	}
	return cloneConstraintReflect(reflect.ValueOf(value)).Interface()
}

func cloneConstraintReflect(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}

	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneConstraintReflect(value.Elem())
		result := reflect.New(value.Type()).Elem()
		result.Set(cloned)
		return result
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.New(value.Type().Elem())
		result.Elem().Set(cloneConstraintReflect(value.Elem()))
		return result
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			result.SetMapIndex(iterator.Key(), cloneConstraintReflect(iterator.Value()))
		}
		return result
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(cloneConstraintReflect(value.Index(i)))
		}
		return result
	case reflect.Array:
		result := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(cloneConstraintReflect(value.Index(i)))
		}
		return result
	case reflect.Struct:
		result := reflect.New(value.Type()).Elem()
		result.Set(value)
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).PkgPath == "" {
				result.Field(i).Set(cloneConstraintReflect(value.Field(i)))
			}
		}
		return result
	default:
		return value
	}
}
