package main

const flagFile = "flag.gen.go"

type flagVariantSpec struct {
	suffix          string
	caller          string
	callArgs        string
	signaturePrefix string
}

func (g *generator) writeFlag(generation generationSpec, spec typeSpec, shorthand bool) {
	variant := flagVariant(shorthand)

	g.l(
		"func ", spec.Name, variant.suffix, "[T ", spec.Constraint,
		"](flagSet *pflag.FlagSet, name string, ", variant.signaturePrefix,
		"target *T, usage string, opts ...Option[T]) {",
	)
	g.l(variant.caller, "(", variant.callArgs, ", ", generation.codecCall(spec), ", opts...)")
	g.l("}")
}

func flagVariant(shorthand bool) flagVariantSpec {
	if shorthand {
		return flagVariantSpec{
			suffix:          "P",
			caller:          "AnyP",
			callArgs:        "flagSet, name, shorthand, target, usage",
			signaturePrefix: "shorthand string, ",
		}
	}

	return flagVariantSpec{
		caller:   "Any",
		callArgs: "flagSet, name, target, usage",
	}
}

func (g generationSpec) codecCall(spec typeSpec) string {
	handler := g.handler(spec.Handler)
	if len(handler.Parameters) == 0 {
		return handler.Name + "[T]()"
	}

	return handler.Name + "[T](" + joinComma(spec.Args) + ")"
}

func (g generationSpec) handler(name string) handlerSpec {
	for _, spec := range g.handlers {
		if spec.Name == name {
			return spec
		}
	}

	panic("unknown handler: " + name)
}
