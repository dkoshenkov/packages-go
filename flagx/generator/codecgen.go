package main

const codecFile = "codec.gen.go"

func (g *generator) codecsFile(spec generationSpec) []byte {
	g.writeHeader(codecImports(spec))
	for i, handler := range spec.handlers {
		if i > 0 {
			g.l()
		}
		g.writeCodec(handler)
	}

	return g.Bytes()
}

func (g *generator) writeCodec(spec handlerSpec) {
	g.l("func ", spec.Name, "[T ", spec.Constraint, "](", joinComma(spec.Parameters), ") Codec[T] {")
	g.l("return Codec[T]{")
	g.l("Parse: func(value string) (T, error) {")
	g.writeLines(spec.ParseLines)
	g.l("},")

	g.l("Format: func(value T) string {")
	g.writeLines(spec.FormatLines)
	g.l("},")

	g.l("Type: ", codecTypeExpr(spec), ",")
	if spec.IsBool {
		g.l("IsBool: true,")
		if spec.NoOptDefVal != "" {
			g.l("NoOptDefVal: ", quote(spec.NoOptDefVal), ",")
		}
	}
	g.l("}")

	g.l("}")
}

func codecTypeExpr(spec handlerSpec) string {
	if spec.UseTypeNameParam {
		return "typeName"
	}

	return quote(spec.TypeName)
}
