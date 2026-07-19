package types

import "reflect"

// CloneValue returns a detached copy of JSON-like data. Provider options,
// schemas, tool arguments, and message content use these shapes throughout the
// SDK, so ownership stays with the domain types instead of individual adapters.
func CloneValue(value any) any {
	if value == nil {
		return nil
	}
	return cloneReflectValue(reflect.ValueOf(value), make(map[cloneVisit]reflect.Value)).Interface()
}

type cloneVisit struct {
	typeOf  reflect.Type
	kind    reflect.Kind
	pointer uintptr
	length  int
}

func cloneReflectValue(src reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
	if !src.IsValid() {
		return src
	}

	switch src.Kind() {
	case reflect.Interface:
		return cloneInterfaceValue(src, visited)
	case reflect.Map:
		return cloneMapValue(src, visited)
	case reflect.Slice:
		return cloneSliceValue(src, visited)
	case reflect.Array:
		dst := reflect.New(src.Type()).Elem()
		for i := 0; i < src.Len(); i++ {
			dst.Index(i).Set(cloneReflectValue(src.Index(i), visited))
		}
		return dst
	case reflect.Pointer:
		return clonePointerValue(src, visited)
	case reflect.Struct:
		return cloneStructValue(src, visited)
	default:
		return src
	}
}

func cloneInterfaceValue(src reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}
	cloned := cloneReflectValue(src.Elem(), visited)
	dst := reflect.New(src.Type()).Elem()
	dst.Set(cloned)
	return dst
}

func cloneMapValue(src reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}
	visit := cloneVisit{typeOf: src.Type(), kind: src.Kind(), pointer: src.Pointer()}
	if dst, found := visited[visit]; found {
		return dst
	}
	dst := reflect.MakeMapWithSize(src.Type(), src.Len())
	visited[visit] = dst
	iterator := src.MapRange()
	for iterator.Next() {
		dst.SetMapIndex(iterator.Key(), cloneReflectValue(iterator.Value(), visited))
	}
	return dst
}

func cloneSliceValue(src reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}
	visit := cloneVisit{typeOf: src.Type(), kind: src.Kind(), pointer: src.Pointer(), length: src.Len()}
	if dst, found := visited[visit]; found {
		return dst
	}
	dst := reflect.MakeSlice(src.Type(), src.Len(), src.Len())
	visited[visit] = dst
	for i := 0; i < src.Len(); i++ {
		dst.Index(i).Set(cloneReflectValue(src.Index(i), visited))
	}
	return dst
}

func clonePointerValue(src reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}
	visit := cloneVisit{typeOf: src.Type(), kind: src.Kind(), pointer: src.Pointer()}
	if dst, found := visited[visit]; found {
		return dst
	}
	dst := reflect.New(src.Type().Elem())
	visited[visit] = dst
	dst.Elem().Set(cloneReflectValue(src.Elem(), visited))
	return dst
}

func cloneStructValue(src reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	dst.Set(src)
	for i := 0; i < src.NumField(); i++ {
		if src.Type().Field(i).IsExported() {
			dst.Field(i).Set(cloneReflectValue(src.Field(i), visited))
		}
	}
	return dst
}
