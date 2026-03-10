package main

type generationSpec struct {
	constraints []constraintSpec
	handlers    []handlerSpec
	types       []typeSpec
}

type constraintSpec struct {
	Name  string
	Terms []string
}

type handlerSpec struct {
	Name             string
	Constraint       string
	Parameters       []string
	Imports          []string
	ParseLines       []string
	FormatLines      []string
	TypeName         string
	UseTypeNameParam bool
	IsBool           bool
}

type typeSpec struct {
	Name       string
	Constraint string
	Handler    string
	Imports    []string
	Args       []string
}

func constraint(name string, terms ...string) constraintSpec {
	return constraintSpec{
		Name:  name,
		Terms: terms,
	}
}

func staticHandler(name, constraint, typeName string, imports, parseLines, formatLines []string, isBool bool) handlerSpec {
	return handlerSpec{
		Name:        name,
		Constraint:  constraint,
		Imports:     imports,
		ParseLines:  parseLines,
		FormatLines: formatLines,
		TypeName:    typeName,
		IsBool:      isBool,
	}
}

func numericHandler(name, constraint, parseLine, formatLine string) handlerSpec {
	return handlerSpec{
		Name:             name,
		Constraint:       constraint,
		Parameters:       list("bitSize int", "typeName string"),
		Imports:          list("strconv"),
		ParseLines:       list(parseLine, "return T(parsed), err"),
		FormatLines:      list(formatLine),
		UseTypeNameParam: true,
	}
}

func flagType(name, constraint, handler string, args ...string) typeSpec {
	return typeSpec{
		Name:       name,
		Constraint: constraint,
		Handler:    handler,
		Args:       args,
	}
}

func importedFlagType(name, constraint, handler string, imports []string, args ...string) typeSpec {
	return typeSpec{
		Name:       name,
		Constraint: constraint,
		Handler:    handler,
		Imports:    imports,
		Args:       args,
	}
}

func newGenerationSpec() generationSpec {
	return generationSpec{
		constraints: []constraintSpec{
			constraint("signedInteger", "~int", "~int8", "~int16", "~int32", "~int64"),
			constraint("unsignedInteger", "~uint", "~uint8", "~uint16", "~uint32", "~uint64"),
			constraint("floatingPoint", "~float32", "~float64"),
		},
		handlers: []handlerSpec{
			staticHandler(
				"stringCodec",
				"~string",
				"string",
				nil,
				list("return T(value), nil"),
				list("return string(value)"),
				false,
			),
			staticHandler(
				"boolCodec",
				"~bool",
				"bool",
				list("strconv"),
				list("parsed, err := strconv.ParseBool(value)", "return T(parsed), err"),
				list("return strconv.FormatBool(bool(value))"),
				true,
			),
			numericHandler(
				"signedIntegerCodec",
				"signedInteger",
				"parsed, err := strconv.ParseInt(value, 0, bitSize)",
				"return strconv.FormatInt(int64(value), 10)",
			),
			numericHandler(
				"unsignedIntegerCodec",
				"unsignedInteger",
				"parsed, err := strconv.ParseUint(value, 0, bitSize)",
				"return strconv.FormatUint(uint64(value), 10)",
			),
			numericHandler(
				"floatingPointCodec",
				"floatingPoint",
				"parsed, err := strconv.ParseFloat(value, bitSize)",
				"return strconv.FormatFloat(float64(value), 'g', -1, bitSize)",
			),
			staticHandler(
				"durationCodec",
				"~int64",
				"duration",
				list("time"),
				list("parsed, err := time.ParseDuration(value)", "return T(parsed), err"),
				list("return time.Duration(value).String()"),
				false,
			),
		},
		types: []typeSpec{
			flagType("String", "~string", "stringCodec"),
			flagType("Bool", "~bool", "boolCodec"),
			importedFlagType("Int", "~int", "signedIntegerCodec", list("strconv"), "strconv.IntSize", quote("int")),
			flagType("Int8", "~int8", "signedIntegerCodec", "8", quote("int8")),
			flagType("Int16", "~int16", "signedIntegerCodec", "16", quote("int16")),
			flagType("Int32", "~int32", "signedIntegerCodec", "32", quote("int32")),
			flagType("Int64", "~int64", "signedIntegerCodec", "64", quote("int64")),
			importedFlagType("Uint", "~uint", "unsignedIntegerCodec", list("strconv"), "strconv.IntSize", quote("uint")),
			flagType("Uint8", "~uint8", "unsignedIntegerCodec", "8", quote("uint8")),
			flagType("Uint16", "~uint16", "unsignedIntegerCodec", "16", quote("uint16")),
			flagType("Uint32", "~uint32", "unsignedIntegerCodec", "32", quote("uint32")),
			flagType("Uint64", "~uint64", "unsignedIntegerCodec", "64", quote("uint64")),
			flagType("Float32", "~float32", "floatingPointCodec", "32", quote("float32")),
			flagType("Float64", "~float64", "floatingPointCodec", "64", quote("float64")),
			flagType("Duration", "~int64", "durationCodec"),
		},
	}
}
