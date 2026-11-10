package research

type Measured struct {
	Score float64
	Bytes int
}

type Objective interface {
	Name() string
	Better(start, best, cand Measured) bool
}

func LookupObjective(name string) (Objective, error) { return nil, nil }

func RegisterObjective(o Objective) {}

func OverLengthCap(startBytes, candBytes int) bool { return false }
