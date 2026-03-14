package main

const constraintsFile = "constraint.gen.go"

func (g *generator) constraintsFile(spec generationSpec) []byte {
	g.writeHeader(nil)
	for i, constraint := range spec.constraints {
		if i > 0 {
			g.l()
		}
		g.writeConstraint(constraint)
	}

	return g.Bytes()
}

func (g *generator) writeConstraint(spec constraintSpec) {
	g.l("type ", spec.Name, " interface {")
	g.l(joinWithPipe(spec.Terms))
	g.l("}")
}
