package configx

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

type fieldSpec struct {
	path       string
	value      reflect.Value
	typ        reflect.Type
	required   bool
	hasDefault bool
	defaultRaw string
	envKey     string
	yamlKey    string
}

type tagSpec struct {
	skip       bool
	name       string
	required   bool
	hasDefault bool
	defaultRaw string
}

func collectFields(value reflect.Value, typ reflect.Type, keyPath []string, structPath []string) ([]fieldSpec, []error) {
	var (
		fields []fieldSpec
		errs   []error
	)

	for i := 0; i < typ.NumField(); i++ {
		fieldType := typ.Field(i)
		tagValue, ok := fieldType.Tag.Lookup(tagCfgx)
		if !ok {
			continue
		}
		if fieldType.PkgPath != "" {
			errs = append(errs, fmt.Errorf("%s: cfgx tag is not allowed on unexported field", fieldType.Name))
			continue
		}

		spec, err := parseCfgxTag(tagValue)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", fieldType.Name, err))
			continue
		}
		if spec.skip {
			continue
		}

		nextKeyPath := appendPath(keyPath, spec.name)
		nextStructPath := appendPath(structPath, fieldType.Name)
		path := strings.Join(nextStructPath, ".")

		fieldValue := value.Field(i)
		nestedValue, nestedType, nested := nestedStructValue(fieldValue, fieldType.Type)
		if nested && hasCfgxTaggedFields(nestedType) {
			if spec.hasDefault {
				errs = append(errs, fmt.Errorf("%s: default is not supported on nested struct field", path))
				continue
			}
			if !spec.required {
				errs = append(errs, fmt.Errorf("%s: optional is not supported on nested struct field", path))
				continue
			}

			nestedFields, nestedErrs := collectFields(nestedValue, nestedType, nextKeyPath, nextStructPath)
			fields = append(fields, nestedFields...)
			errs = append(errs, nestedErrs...)
			continue
		}

		if !isSupportedType(fieldType.Type) {
			errs = append(errs, fmt.Errorf("%s: unsupported type %s", path, fieldType.Type))
			continue
		}

		envKey := strings.TrimSpace(fieldType.Tag.Get(tagEnv))
		if envKey == "" {
			envKey = buildEnvKey(nextKeyPath)
		}

		yamlKey := strings.TrimSpace(fieldType.Tag.Get(tagYAML))
		if yamlKey == "" {
			yamlKey = buildYAMLKey(nextKeyPath)
		}

		fields = append(fields, fieldSpec{
			path:       path,
			value:      fieldValue,
			typ:        fieldType.Type,
			required:   spec.required,
			hasDefault: spec.hasDefault,
			defaultRaw: spec.defaultRaw,
			envKey:     envKey,
			yamlKey:    yamlKey,
		})
	}

	return fields, errs
}

func assignResolvedValue(field fieldSpec, resolved resolvedValue) error {
	if resolved.raw != nil {
		decoded, err := decodeYAMLBytes([]byte(*resolved.raw), field.typ)
		if err != nil {
			return fmt.Errorf("%s: decode value from %s (%s): %w", field.path, resolved.source, resolved.key, err)
		}
		field.value.Set(decoded)
		return nil
	}

	decoded, err := decodeYAMLValue(resolved.any, field.typ)
	if err != nil {
		return fmt.Errorf("%s: decode value from %s (%s): %w", field.path, resolved.source, resolved.key, err)
	}
	field.value.Set(decoded)

	return nil
}

func assignDefault(field fieldSpec) error {
	decoded, err := decodeYAMLBytes([]byte(field.defaultRaw), field.typ)
	if err != nil {
		return fmt.Errorf("%s: decode default: %w", field.path, err)
	}
	field.value.Set(decoded)

	return nil
}

func decodeYAMLValue(value any, targetType reflect.Type) (reflect.Value, error) {
	content, err := yaml.Marshal(value)
	if err != nil {
		return reflect.Value{}, err
	}

	return decodeYAMLBytes(content, targetType)
}

func decodeYAMLBytes(content []byte, targetType reflect.Type) (reflect.Value, error) {
	target := reflect.New(targetType)
	if err := yaml.Unmarshal(content, target.Interface()); err != nil {
		return reflect.Value{}, err
	}

	return target.Elem(), nil
}

func parseCfgxTag(tag string) (tagSpec, error) {
	value := strings.TrimSpace(tag)
	if value == "" {
		return tagSpec{}, errCfgxTagEmpty
	}
	if value == cfgxSkipValue {
		return tagSpec{skip: true}, nil
	}

	parts := strings.Split(value, ",")
	name := strings.TrimSpace(parts[0])
	if name == "" {
		return tagSpec{}, errCfgxKeyEmpty
	}

	spec := tagSpec{
		name:     name,
		required: true,
	}

	for _, part := range parts[1:] {
		option := strings.TrimSpace(part)
		if option == "" {
			continue
		}

		switch {
		case option == cfgxOptionOptional:
			spec.required = false
		case option == cfgxOptionRequired:
			spec.required = true
		case strings.HasPrefix(option, cfgxOptionDefault):
			if spec.hasDefault {
				return tagSpec{}, errCfgxDuplicatedDefault
			}
			spec.hasDefault = true
			spec.defaultRaw = strings.TrimSpace(strings.TrimPrefix(option, cfgxOptionDefault))
		default:
			return tagSpec{}, fmt.Errorf("cfgx tag has unknown option %q", option)
		}
	}

	return spec, nil
}

func nestedStructValue(fieldValue reflect.Value, fieldType reflect.Type) (reflect.Value, reflect.Type, bool) {
	switch fieldType.Kind() {
	case reflect.Struct:
		return fieldValue, fieldType, true
	case reflect.Ptr:
		if fieldType.Elem().Kind() != reflect.Struct {
			return reflect.Value{}, nil, false
		}
		if fieldValue.IsNil() && fieldValue.CanSet() {
			fieldValue.Set(reflect.New(fieldType.Elem()))
		}
		if fieldValue.IsNil() {
			return reflect.Value{}, nil, false
		}
		return fieldValue.Elem(), fieldType.Elem(), true
	default:
		return reflect.Value{}, nil, false
	}
}

func hasCfgxTaggedFields(typ reflect.Type) bool {
	for i := 0; i < typ.NumField(); i++ {
		if _, ok := typ.Field(i).Tag.Lookup(tagCfgx); ok {
			return true
		}
	}

	return false
}

func isSupportedType(typ reflect.Type) bool {
	switch typ.Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return false
	default:
		return true
	}
}

func buildEnvKey(parts []string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized = append(normalized, normalizeEnvSegment(part))
	}
	return strings.Join(normalized, envPartsSeparator)
}

func normalizeEnvSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var b strings.Builder
	var prev rune
	for i, current := range value {
		if unicode.IsLetter(current) || unicode.IsDigit(current) {
			if unicode.IsUpper(current) && i > 0 && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToUpper(current))
			prev = current
			continue
		}

		if b.Len() > 0 && prev != '_' {
			b.WriteRune('_')
			prev = '_'
		}
	}

	return strings.Trim(b.String(), "_")
}

func buildYAMLKey(parts []string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized = append(normalized, strings.TrimSpace(part))
	}
	return strings.Join(normalized, yamlPartsSeparator)
}

func appendPath(parts []string, value string) []string {
	result := make([]string, 0, len(parts)+1)
	result = append(result, parts...)
	result = append(result, value)
	return result
}

func normalizeKey(value string) string {
	return buildEnvKey([]string{value})
}
