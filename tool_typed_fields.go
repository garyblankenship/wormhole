package wormhole

import (
	"reflect"
	"sort"
	"strings"
	"unicode"
)

type schemaJSONField struct {
	name   string
	field  reflect.StructField
	depth  int
	tagged bool
	order  int
}

// schemaJSONFields follows encoding/json's dominance rules: the shallowest
// field wins, a sole tagged field wins ties, and unresolved ties disappear.
func schemaJSONFields(t reflect.Type) []schemaJSONField {
	var candidates []schemaJSONField
	var visit func(reflect.Type, int, map[reflect.Type]bool)
	visit = func(current reflect.Type, depth int, ancestors map[reflect.Type]bool) {
		if ancestors[current] {
			return
		}
		ancestors[current] = true
		defer delete(ancestors, current)
		for i := 0; i < current.NumField(); i++ {
			f := current.Field(i)
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if !f.IsExported() && (!f.Anonymous || ft.Kind() != reflect.Struct) {
				continue
			}
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name := strings.Split(tag, ",")[0]
			if !validSchemaJSONName(name) {
				name = ""
			}
			if f.Anonymous && name == "" && ft.Kind() == reflect.Struct {
				visit(ft, depth+1, ancestors)
				continue
			}
			tagged := name != ""
			if name == "" {
				name = f.Name
			}
			candidates = append(candidates, schemaJSONField{name, f, depth, tagged, len(candidates)})
		}
	}
	visit(t, 0, make(map[reflect.Type]bool))
	groups := make(map[string][]schemaJSONField)
	for _, f := range candidates {
		groups[f.name] = append(groups[f.name], f)
	}
	var selected []schemaJSONField
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].depth != group[j].depth {
				return group[i].depth < group[j].depth
			}
			return group[i].tagged && !group[j].tagged
		})
		if len(group) > 1 && group[0].depth == group[1].depth && group[0].tagged == group[1].tagged {
			continue
		}
		selected = append(selected, group[0])
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].order < selected[j].order })
	return selected
}

func validSchemaJSONName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c) || unicode.IsLetter(c) || unicode.IsDigit(c) {
			continue
		}
		return false
	}
	return true
}
